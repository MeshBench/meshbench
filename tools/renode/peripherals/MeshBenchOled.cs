//
// A board's monochrome OLED, under Renode: an SSD1306 on I2C.
//
// The RAK4631's display, and the last board panel that was not drawn (#602).
// It registers on the TWIM at its address, so the controller answers the
// address rather than NACKing it, and every transfer's bytes arrive here.
//
// The part's protocol on I2C: each transfer starts with a control byte. 0x00
// says commands follow, 0x40 that display data does, and a set bit 7 (0x80,
// 0xC0) that only one byte follows before another control byte. The driver
// (Adafruit_SSD1306) sets a column and page window and then streams the
// buffer in, so that is what is modelled: the addressing commands, display on
// and off, and the data. Contrast, charge pump, multiplex and the rest change
// how the glass looks, not what is on it, and are consumed with their
// arguments and otherwise ignored.
//
// Like MeshBenchPanel, nothing here improves on the input. The frame is the
// controller's own memory, page-addressed as the part lays it out - a byte per
// column per page of eight rows, bit 0 at the top - and it is not sent until
// the firmware has written something: nothing drawn is not black.
//
using System;
using System.Collections.Generic;
using System.Net.Sockets;
using System.Threading;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals.I2C;

namespace Antmicro.Renode.Peripherals.Video
{
    public class MeshBenchOled : II2CPeripheral, IDisposable
    {
        public MeshBenchOled(int port, int width = 128, int height = 64, int columnOffset = 0)
        {
            this.port = port;
            this.width = width;
            this.height = height;
            this.columnOffset = columnOffset;
            pages = height / 8;
            ram = new byte[width * pages];
            Reset();
            Start();
        }

        public void Reset()
        {
            Array.Clear(ram, 0, ram.Length);
            colStart = 0;
            colEnd = width - 1;
            pageStart = 0;
            pageEnd = pages - 1;
            col = 0;
            page = 0;
            horizontal = false;
            on = false;
            drawn = false;
            pending.Clear();
            need = 0;
        }

        // One I2C write: a control byte, then commands or data.
        public void Write(byte[] data)
        {
            var i = 0;
            while(i < data.Length)
            {
                var control = data[i++];
                var single = (control & 0x80) != 0;
                var isData = (control & 0x40) != 0;
                if(single)
                {
                    if(i < data.Length)
                    {
                        Byte(data[i++], isData);
                    }
                    continue;
                }
                while(i < data.Length)
                {
                    Byte(data[i++], isData);
                }
            }
        }

        // The SSD1306 can be read only for its status byte, which says the
        // display is on in bit 6 inverted. Nothing MeshCore drives reads it.
        public byte[] Read(int count = 1)
        {
            var status = (byte)(on ? 0x00 : 0x40);
            var answer = new byte[count];
            for(var k = 0; k < count; k++)
            {
                answer[k] = status;
            }
            return answer;
        }

        public void FinishTransmission() { }

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

        private void Byte(byte b, bool isData)
        {
            if(isData)
            {
                Data(b);
                return;
            }
            if(need > 0)
            {
                pending.Add(b);
                need--;
                if(need == 0)
                {
                    Apply();
                }
                return;
            }
            Command(b);
        }

        private void Command(byte b)
        {
            pending.Clear();
            pending.Add(b);
            need = ArgsOf(b);
            if(need == 0)
            {
                Apply();
            }
        }

        private static int ArgsOf(byte b)
        {
            switch(b)
            {
            case 0x20: case 0x81: case 0x8D: case 0xA8: case 0xD3:
            case 0xD5: case 0xD9: case 0xDA: case 0xDB:
                return 1;
            case 0x21: case 0x22: case 0xA3:
                return 2;
            case 0x29: case 0x2A:
                return 5;
            case 0x26: case 0x27:
                return 6;
            default:
                return 0;
            }
        }

        private void Apply()
        {
            var b = pending[0];
            switch(b)
            {
            case 0x20:
                // Memory addressing mode: 0 horizontal, 1 vertical, 2 page. The
                // driver uses horizontal; page mode is what the part resets to.
                horizontal = (pending[1] & 0x03) == 0;
                break;
            case 0x21:
                colStart = Clamp(pending[1] - columnOffset, width - 1);
                colEnd = Clamp(pending[2] - columnOffset, width - 1);
                col = colStart;
                break;
            case 0x22:
                pageStart = Clamp(pending[1] & 0x07, pages - 1);
                pageEnd = Clamp(pending[2] & 0x07, pages - 1);
                page = pageStart;
                break;
            case 0xAE:
                on = false;
                Send();
                break;
            case 0xAF:
                on = true;
                Send();
                break;
            default:
                if(b >= 0xB0 && b <= 0xB7)
                {
                    page = Clamp(b & 0x07, pages - 1); // page mode: page start
                }
                else if(b <= 0x0F)
                {
                    col = Clamp((col & 0xF0) | b, width - 1);
                }
                else if(b >= 0x10 && b <= 0x1F)
                {
                    col = Clamp(((b & 0x0F) << 4) | (col & 0x0F), width - 1);
                }
                break;
            }
            pending.Clear();
        }

        private static int Clamp(int v, int max)
        {
            return v < 0 ? 0 : (v > max ? max : v);
        }

        private void Data(byte b)
        {
            ram[page * width + col] = b;
            drawn = true;
            if(!horizontal)
            {
                // Page mode: the column advances and wraps within the page.
                col = col >= width - 1 ? 0 : col + 1;
                return;
            }
            col++;
            if(col > colEnd)
            {
                col = colStart;
                page++;
                if(page > pageEnd)
                {
                    page = pageStart;
                    // The window is full: the firmware has finished drawing.
                    Send();
                }
            }
        }

        private void Start()
        {
            running = true;
            thread = new Thread(Serve) { IsBackground = true, Name = "meshbench-oled" };
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
                    this.Log(LogLevel.Debug, "oled: not connected yet: {0}", e.Message);
                    Thread.Sleep(200);
                    continue;
                }
                this.Log(LogLevel.Info, "oled: {0}x{1} connected on port {2}", width, height, port);
                return;
            }
        }

        // One frame in the shape the workbench reads: MBF2, the size, one bit
        // a pixel, whether the panel is on, then the controller's memory.
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
                1, (byte)(on ? 1 : 0),
            };
            lock(sendLock)
            {
                try
                {
                    var wire = c.GetStream();
                    wire.Write(header, 0, header.Length);
                    wire.Write(ram, 0, ram.Length);
                }
                catch(Exception)
                {
                    client = null;
                }
            }
        }

        private readonly int port;
        private readonly int width;
        private readonly int height;
        private readonly int pages;
        private readonly int columnOffset;
        private readonly byte[] ram;
        private readonly List<byte> pending = new List<byte>();
        private readonly object sendLock = new object();

        private int need;
        private int colStart, colEnd, pageStart, pageEnd, col, page;
        private bool horizontal;
        private bool on;
        private bool drawn;

        private volatile bool running;
        private TcpClient client;
        private Thread thread;
    }
}
