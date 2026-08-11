//
// The SX1262, as seen from Renode — a wire to the chip, not a model of one.
//
// There was a model here before: sixteen kilobytes of C# implementing the
// command protocol, the status byte, IRQ flags and the buffer. It worked, and
// it was the wrong shape. The QEMU backend needed the same thing and already
// had VirtualSX1262, so finishing this one would have left two implementations
// of one chip that had to agree for ever. They do not agree for ever. The first
// time they drifted, every comparison between an ARM node and an ESP32 node
// would have been measuring our own code rather than MeshCore's.
//
// So this forwards SPI to the same radioserver process the emulated ESP32 talks
// to, which owns the same VirtualSX1262 a native node reaches in process. One
// chip, three ways in.
//
//   Renode ---\
//              >--- radioserver --- VirtualSX1262 --- the RF engine
//   QEMU   ---/
//
// TCP rather than a Unix socket because Renode runs on Mono, whose Unix domain
// socket support has been unreliable for long enough that betting a node on it
// is a poor trade for one path separator.
//
using System;
using System.Net.Sockets;
using Antmicro.Renode.Core;
using Antmicro.Renode.Core.Structure;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals.SPI;

namespace Antmicro.Renode.Peripherals.Radio
{
    public class RadioServerSX1262 : ISPIPeripheral, IGPIOReceiver
    {
        public RadioServerSX1262(IMachine machine, string host = "127.0.0.1", int port = 0)
        {
            this.machine = machine;
            this.host = host;
            this.port = port;
        }

        // Connect when the script says so rather than in the constructor: a
        // platform description is loaded before the radio model is running, and
        // a peripheral that throws while the machine is being built takes the
        // whole machine with it.
        public void Connect()
        {
            if(port == 0)
            {
                this.Log(LogLevel.Error,
                    "no radio model port given; this node has no radio and will " +
                    "report chip-not-found rather than say so");
                return;
            }
            try
            {
                client = new TcpClient(host, port) { NoDelay = true };
                stream = client.GetStream();
                this.Log(LogLevel.Info, "chip attached via radio model at {0}:{1}", host, port);
            }
            catch(Exception e)
            {
                // Loud. An unattached radio answers every register read with
                // zero, which RadioLib reports as no chip present — a wiring
                // error dressed as a hardware fault.
                this.Log(LogLevel.Error, "no radio model at {0}:{1} — {2}", host, port, e.Message);
                client = null;
                stream = null;
            }
        }

        // Chip select. RadioLib drives NSS as an ordinary GPIO rather than
        // letting the SPI controller do it, which is why this is an
        // IGPIOReceiver as well: the transaction boundary arrives on a pin, not
        // from the bus. Getting this wrong does not fail loudly — the chip sees
        // one unframed byte stream and answers nothing that makes sense.
        public void OnGPIO(int number, bool value)
        {
            if(number != NssPin)
            {
                return;
            }
            // Active low, as on the part.
            if(!value)
            {
                Send(CsAssert);
                selected = true;
            }
            else if(selected)
            {
                Send(CsRelease);
                selected = false;
            }
        }

        public byte Transmit(byte data)
        {
            if(stream == null)
            {
                return 0;
            }
            try
            {
                // The controller may clock bytes without ever touching NSS, if
                // the platform wires chip select to the peripheral itself. Open
                // a transaction rather than dropping the byte: a silent zero
                // here is the hardest kind of fault to find.
                if(!selected)
                {
                    Send(CsAssert);
                    selected = true;
                    implicitSelect = true;
                }
                stream.WriteByte(Xfer);
                stream.WriteByte(data);
                var got = stream.ReadByte();
                if(got < 0)
                {
                    Drop("the radio model closed the connection");
                    return 0;
                }
                return (byte)got;
            }
            catch(Exception e)
            {
                Drop(e.Message);
                return 0;
            }
        }

        public void FinishTransmission()
        {
            if(selected && implicitSelect)
            {
                Send(CsRelease);
                selected = false;
                implicitSelect = false;
            }
        }

        // BUSY, read as a GPIO by RadioLib between commands. Always clear, which
        // is what the native path and the QEMU path both answer: VirtualSX1262
        // does not model the time a real chip spends digesting a command, and
        // answering differently here would make an ARM node a different radio
        // from every other node in the scenario.
        public bool Busy
        {
            get { return false; }
        }

        public void Reset()
        {
            selected = false;
            implicitSelect = false;
        }

        private void Send(byte tag)
        {
            if(stream == null)
            {
                return;
            }
            try
            {
                stream.WriteByte(tag);
            }
            catch(Exception e)
            {
                Drop(e.Message);
            }
        }

        private void Drop(string why)
        {
            this.Log(LogLevel.Error, "radio model went away — {0}", why);
            stream = null;
            client = null;
            selected = false;
        }

        // The emulator side of the radio model's protocol. Four tags, because
        // this is on the hot path of every SPI byte.
        private const byte CsAssert = 0x01;
        private const byte CsRelease = 0x02;
        private const byte Xfer = 0x03;
        private const byte ReadBusy = 0x04;

        // Which GPIO carries chip select into this peripheral. Renode numbers
        // the connections a platform declares, so this is the index in the
        // .repl rather than a pin on the board.
        private const int NssPin = 0;

        private readonly IMachine machine;
        private readonly string host;
        private readonly int port;

        private TcpClient client;
        private NetworkStream stream;
        private bool selected;
        private bool implicitSelect;
    }
}
