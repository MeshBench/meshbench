//
// A board's colour panel, under Renode.
//
// The ST7789 and the ST7735 are the two controllers the nRF52 boards here
// carry, and for the purpose of showing what the firmware drew they are the
// same part: address a window with CASET and RASET, then stream RGB565 into it
// with RAMWR. Everything else a real controller does - gamma tables, porch
// timing, tearing, the frame-rate divider - changes what a panel looks like and
// not what is on it, so none of it is modelled and the picture is honest about
// being the firmware's own output rather than a photograph.
//
// This does not decide anything about the picture. It records the bytes the
// firmware sent to the addresses it sent them to, and hands them on unchanged.
// A panel model that improved on its input - scaled, smoothed, filled in a
// missing region - would be showing something no firmware drew, which is worse
// than showing nothing.
//
// It is alone on its controller, because that is where the board puts it: both
// Heltec boards here drive their panel from the Arduino core's SPI1, not from
// the bus the radio is on. So nothing has to be told apart - the controller
// clocks bytes at one device, and the command/data line says which kind each
// byte is.
//
using System;
using System.Net.Sockets;
using System.Threading;
using Antmicro.Renode.Core;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals;
using Antmicro.Renode.Peripherals.SPI;

namespace Antmicro.Renode.Peripherals.Video
{
    public class MeshBenchPanel : ISPIPeripheral, IGPIOReceiver, IDisposable
    {
        // width and height are the glass, from the board's own profile. dcPin is
        // the command/data line: this part cannot tell a command from a pixel by
        // looking at it, which is the whole reason that line exists.
        public MeshBenchPanel(int port, int width, int height, int csPin, int dcPin)
        {
            this.port = port;
            this.width = width;
            this.height = height;
            this.csPin = csPin;
            this.dcPin = dcPin;
            pixels = new byte[width * height * 2];
            // Nothing drawn is not the same as black. Until the firmware writes
            // a pixel the frame is not sent at all, so the window says "nothing
            // drawn yet" rather than showing a black screen the board never
            // produced.
            Start();
        }

        public void Reset()
        {
            command = 0;
            argsSeen = 0;
            writing = false;
            on = false;
            drawn = false;
            Array.Clear(pixels, 0, pixels.Length);
        }

        public void Dispose()
        {
            running = false;
            var c = client;
            client = null;
            if(c != null)
            {
                try { c.Close(); } catch(Exception) { }
            }
        }

        // The command/data line, and chip select. Both are ordinary GPIOs the
        // driver works itself rather than lines the controller manages, which is
        // why they arrive here as pins.
        //
        // A rising select does not end a pixel run. TFT_eSPI raises it between
        // the window and the pixels of the same update, and a model that reset
        // its state here would lose every frame after the first row - so it
        // ends an argument only, which is the thing a partial command leaves
        // behind.
        public void OnGPIO(int number, bool value)
        {
            if(number == dcPin)
            {
                data = value;
                return;
            }
            if(number == csPin && value)
            {
                argsSeen = 0;
            }
        }

        public byte Transmit(byte value)
        {
            if(!data)
            {
                Command(value);
                return 0;
            }
            if(writing)
            {
                Pixel(value);
                return 0;
            }
            Argument(value);
            return 0;
        }

        public void FinishTransmission() { }

        private void Command(byte code)
        {
            command = code;
            argsSeen = 0;
            switch(code)
            {
            case RamWr:
                // The window has been set; pixels follow until the next command.
                writing = true;
                x = xs;
                y = ys;
                break;
            case DispOn:
                writing = false;
                on = true;
                Send();
                break;
            case DispOff:
                // Said rather than blanked. MeshCore switches the panel off
                // after an idle and the board's own button brings it back, so a
                // dark frame and a sleeping one are different facts and the
                // window draws them differently.
                writing = false;
                on = false;
                Send();
                break;
            default:
                writing = false;
                break;
            }
        }

        // CASET and RASET each carry two sixteen-bit addresses, high byte first,
        // and they are absolute positions in the controller's memory rather than
        // on the glass.
        private void Argument(byte value)
        {
            if(command != ColAddr && command != RowAddr)
            {
                return;
            }
            if(argsSeen < args.Length)
            {
                args[argsSeen] = value;
            }
            argsSeen++;
            if(argsSeen != args.Length)
            {
                return;
            }
            argsSeen = 0;
            var start = (args[0] << 8) | args[1];
            var end = (args[2] << 8) | args[3];
            if(command == ColAddr)
            {
                xs = start;
                xe = end;
            }
            else
            {
                ys = start;
                ye = end;
            }
            // A panel smaller than its controller's memory sits at an offset
            // inside it, and the driver adds that offset before it reaches the
            // wire - so the addresses here are not zero-based on the glass.
            //
            // Learned from what the firmware actually addressed rather than
            // carried as a constant per board. TFT_eSPI clears the whole panel
            // at start-up, so the smallest address ever seen is the offset, and
            // a wrong answer here shifts the picture visibly rather than
            // corrupting it quietly. A constant transcribed per controller and
            // per rotation would be four chances to be silently wrong.
            var moved = false;
            if(start < origin(command))
            {
                if(command == ColAddr)
                {
                    originX = start;
                }
                else
                {
                    originY = start;
                }
                moved = true;
            }
            if(moved)
            {
                Array.Clear(pixels, 0, pixels.Length);
            }
        }

        private int origin(byte which) => which == ColAddr ? originX : originY;

        // RGB565, high byte first on the wire. Stored low byte first, because
        // that is the order the frame this sends is defined in and both ends of
        // the socket are the same machine.
        private void Pixel(byte value)
        {
            if(!high)
            {
                pending = value;
                high = true;
                return;
            }
            high = false;
            var px = x - originX;
            var py = y - originY;
            if(px >= 0 && py >= 0 && px < width && py < height)
            {
                var i = (py * width + px) * 2;
                pixels[i] = value;
                pixels[i + 1] = pending;
                drawn = true;
            }
            // The controller wraps within the addressed window, not the panel.
            x++;
            if(x > xe)
            {
                x = xs;
                y++;
                if(y > ye)
                {
                    y = ys;
                }
            }
            // A frame is sent when a window is filled rather than on a timer:
            // that is the moment the firmware finished drawing something, and a
            // timer would catch half-written pictures and report tearing this
            // model has no business inventing.
            if(x == xs && y == ys)
            {
                Send();
            }
        }

        private void Start()
        {
            running = true;
            thread = new Thread(Serve) { IsBackground = true, Name = "meshbench-panel" };
            thread.Start();
        }

        private void Serve()
        {
            while(running)
            {
                try
                {
                    client = new TcpClient("127.0.0.1", port) { NoDelay = true };
                }
                catch(Exception e)
                {
                    if(!running)
                    {
                        return;
                    }
                    this.Log(LogLevel.Debug, "panel: not connected yet: {0}", e.Message);
                    Thread.Sleep(RetryMs);
                    continue;
                }
                this.Log(LogLevel.Info, "panel: {0}x{1} connected on port {2}",
                         width, height, port);
                return;
            }
        }

        // One frame, header and pixels, in the shape the workbench reads.
        //
        // Dropped rather than queued when the socket is not there or will not
        // take it: a picture is only ever the latest one, and a model that
        // blocked here would stop the machine it is modelling.
        private void Send()
        {
            if(!drawn)
            {
                return;
            }
            var c = client;
            if(c == null)
            {
                return;
            }
            var header = new byte[]
            {
                (byte)'M', (byte)'B', (byte)'F', (byte)'2',
                (byte)(width & 0xFF), (byte)((width >> 8) & 0xFF),
                (byte)(height & 0xFF), (byte)((height >> 8) & 0xFF),
                16, (byte)(on ? 1 : 0),
            };
            lock(sendLock)
            {
                try
                {
                    var wire = c.GetStream();
                    wire.Write(header, 0, header.Length);
                    wire.Write(pixels, 0, pixels.Length);
                }
                catch(Exception)
                {
                    client = null;
                }
            }
        }

        private const byte ColAddr = 0x2A;
        private const byte RowAddr = 0x2B;
        private const byte RamWr = 0x2C;
        private const byte DispOff = 0x28;
        private const byte DispOn = 0x29;
        private const int RetryMs = 200;

        private readonly int port;
        private readonly int width;
        private readonly int height;
        private readonly int csPin;
        private readonly int dcPin;
        private readonly byte[] pixels;
        private readonly byte[] args = new byte[4];
        private readonly object sendLock = new object();

        private byte command;
        private int argsSeen;
        private bool data;
        private bool writing;
        private bool high;
        private byte pending;
        private bool on;
        private bool drawn;

        private int xs, xe, ys, ye, x, y;
        private int originX = int.MaxValue;
        private int originY = int.MaxValue;

        private volatile bool running;
        private TcpClient client;
        private Thread thread;
    }
}
