//
// The far end of a board's buttons, under Renode.
//
// A board profile declares what somebody can press on it, and the workbench
// draws whatever it declared. That declaration is worth nothing on its own: a
// button nothing drives is a control that reports success and changes no pin,
// which is indistinguishable from firmware that ignores the button - and the
// second is a bug somebody would go looking for.
//
// The QEMU boards have had this since the T-Deck: a device inside the machine
// reads a socket the workbench listens on and moves the pins. This is the same
// device for the other emulator, reading the same eight-byte messages, so a
// press on a Heltec T114 and a press on a T-Deck differ in nothing above this
// file.
//
// TCP rather than a socket file because Renode offers nothing else, which is
// the same reason its console is on a port.
//
// What it does not do is watch. Lamps are outputs, and reporting one means
// following a GPIO the other way; neither machine does that yet, so both say
// so rather than one of them guessing.
//
using System;
using System.Net.Sockets;
using System.Threading;
using Antmicro.Renode.Core;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals;
using Antmicro.Renode.Peripherals.Bus;

namespace Antmicro.Renode.Peripherals.Miscellaneous
{
    // The converter this feeds, named as an interface so the input channel does
    // not have to know which model is registered - and so a board with no
    // converter can leave it out.
    public interface IMeshBenchMeter : IPeripheral
    {
        uint Sample { get; set; }
    }

    // On the bus at an address nothing on this chip uses, because that is how a
    // .repl registers a peripheral and there is no lighter way to declare one.
    // Nothing reads or writes it: the guest does not know it is there, which is
    // correct - a person pressing a button is not something firmware can query.
    public class MeshBenchInputs : IDoubleWordPeripheral, IKnownSize, IDisposable
    {
        // The two GPIO ports of an nRF52840 and, optionally, the converter its
        // cell is read through. Ports are required: a board with no buttons
        // does not get one of these at all.
        public MeshBenchInputs(int port, IGPIOReceiver gpio0, IGPIOReceiver gpio1,
                               IMeshBenchMeter meter = null)
        {
            this.port = port;
            this.gpio0 = gpio0;
            this.gpio1 = gpio1;
            this.meter = meter;
            Start();
        }

        public long Size => 0x100;

        public uint ReadDoubleWord(long offset) => 0;

        public void WriteDoubleWord(long offset, uint value)
        {
        }

        public void Reset()
        {
            // Deliberately nothing. A reset is the board restarting, and the
            // workbench is still holding the same buttons it was holding - it
            // re-sends what is held when the connection is remade, and this end
            // has no opinion about the levels in between.
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

        private void Start()
        {
            running = true;
            thread = new Thread(Serve)
            {
                IsBackground = true,
                Name = "meshbench-inputs"
            };
            thread.Start();
        }

        // One connection, retried until the machine goes away.
        //
        // The workbench is listening before Renode starts, so the first attempt
        // normally succeeds; the retry is for a board brought up while the
        // workbench is still opening its sockets, which is a race nobody should
        // have to think about.
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
                    this.Log(LogLevel.Debug, "inputs: not connected yet: {0}", e.Message);
                    Thread.Sleep(RetryMs);
                    continue;
                }
                this.Log(LogLevel.Info, "inputs: connected on port {0}", port);
                Read(client.GetStream());
                if(running)
                {
                    this.Log(LogLevel.Warning, "inputs: the workbench let go of this board's buttons");
                }
                return;
            }
        }

        private void Read(NetworkStream wire)
        {
            var message = new byte[MessageLength];
            while(running)
            {
                var at = 0;
                while(at < MessageLength)
                {
                    int got;
                    try
                    {
                        got = wire.Read(message, at, MessageLength - at);
                    }
                    catch(Exception)
                    {
                        return;
                    }
                    if(got <= 0)
                    {
                        return;
                    }
                    at += got;
                }
                Apply(message);
            }
        }

        // Every message is a tag and seven bytes of payload, fixed width so a
        // reader can skip what is not its own without knowing how long it was.
        private void Apply(byte[] message)
        {
            switch((char)message[0])
            {
            case 'B':
                Drive(message[1], message[2] != 0);
                break;
            case 'A':
                var raw = (uint)(message[2] | (message[3] << 8));
                if(meter == null)
                {
                    this.Log(LogLevel.Debug, "inputs: a reading for a converter this board has not got");
                    break;
                }
                meter.Sample = raw;
                break;
            case 'K':
            case 'T':
                // A keyboard and a touch panel, which no board this emulator
                // runs carries. Skipped rather than logged: the workbench sends
                // to every device on the channel and each keeps its own.
                break;
            default:
                this.Log(LogLevel.Warning, "inputs: a message tagged '{0}'", (char)message[0]);
                break;
            }
        }

        // Pins arrive in the flat numbering the nRF52 Arduino core and every
        // board profile use: P0.x is x and P1.x is 32+x. Renode names the two
        // ports separately, so this is where the two conventions meet, in one
        // place rather than in each board's wiring.
        private void Drive(int pin, bool level)
        {
            var line = pin < PinsPerPort ? gpio0 : gpio1;
            // Straight onto the port from this reader's own thread, which is
            // what the monitor does for the pins a board holds high at bring-up
            // - "gpio1 OnGPIO 10 true" in a node's own script, running on
            // Renode's monitor thread and not on the emulation one.
            line.OnGPIO(pin % PinsPerPort, level);
        }

        private const int MessageLength = 8;
        private const int PinsPerPort = 32;
        private const int RetryMs = 200;

        private readonly int port;
        private readonly IGPIOReceiver gpio0;
        private readonly IGPIOReceiver gpio1;
        private readonly IMeshBenchMeter meter;

        private volatile bool running;
        private TcpClient client;
        private Thread thread;
    }
}
