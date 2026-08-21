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
using Antmicro.Renode.Peripherals.Timers;
using Antmicro.Renode.Time;

namespace Antmicro.Renode.Peripherals.Radio
{
    public class RadioServerSX1262 : ISPIPeripheral, IGPIOReceiver
    {
        public RadioServerSX1262(IMachine machine, string host = "127.0.0.1", int port = 0)
        {
            this.machine = machine;
            this.host = host;
            this.port = port;

            // DIO1, polled. The chip is in another process, so there is nothing
            // to push an interrupt across - and the alternative, having the
            // firmware poll the IRQ register over SPI, is what the ESP32 build
            // does but the nRF52 one does not. It waits on the pin.
            //
            // A kilohertz is far finer than anything the radio times: the
            // shortest thing DIO1 signals is a preamble detection, tens of
            // milliseconds at these spreading factors.
            irqPoll = new LimitTimer(machine.ClockSource, 1000, this, "dio1",
                                     limit: 1, eventEnabled: true,
                                     direction: Direction.Ascending,
                                     enabled: false, workMode: WorkMode.Periodic);
            irqPoll.LimitReached += PollIrq;
        }

        // DIO1 into the MCU. Wired in the platform description to the pin the
        // board uses - P1.15 on a RAK4631.
        public GPIO IRQ { get; } = new GPIO();

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
                irqPoll.Enabled = true;
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
            lock(wire)
            {
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

        // Ask the chip whether DIO1 is asserted, and drive the pin to match.
        private void PollIrq()
        {
            if(stream == null)
            {
                return;
            }
            lock(wire)
            {
                try
                {
                    stream.WriteByte(ReadIrq);
                    var got = stream.ReadByte();
                    if(got < 0)
                    {
                        Drop("the radio model closed the connection");
                        return;
                    }
                    var asserted = got != 0;
                    if(asserted != irqLine)
                    {
                        irqLine = asserted;
                        IRQ.Set(asserted);
                        // Every edge, because the question "does a received
                        // packet ever reach the firmware" is answered here and
                        // nowhere else on this side of the socket. An emulated
                        // board that transmits on its own timer and never acts
                        // on anything looks identical, from the engine's side,
                        // to one that is simply being ignored - the engine
                        // records a reception because the channel delivered it,
                        // which is a different claim.
                        //
                        // Edges only, so a chip that is quiet costs nothing.
                        edges++;
                        if(edges <= EdgeBudget)
                        {
                            this.Log(LogLevel.Warning, "sx1262 irq {0} (edge {1}){2}",
                                asserted ? "high" : "low", edges,
                                edges == EdgeBudget ? "  (quiet from here)" : string.Empty);
                        }
                    }
                }
                catch(Exception e)
                {
                    Drop(e.Message);
                }
            }
        }

        private void Send(byte tag)
        {
            if(stream == null)
            {
                return;
            }
            lock(wire)
            {
                try
                {
                    stream.WriteByte(tag);
                }
                catch(Exception e)
                {
                    Drop(e.Message);
                }
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
        private const byte ReadIrq = 0x05;

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
        private bool irqLine;
        // A working chip raises a couple of edges a packet, so a run's worth is
        // a few hundred lines. Enough to see the shape and then quiet.
        private const int EdgeBudget = 40;

        private int edges;
        private readonly LimitTimer irqPoll;
        // One socket, two threads: SPI arrives on the CPU thread and the DIO1
        // poll on a timer. Interleaving a tag with a transfer would desync the
        // stream and read one answer as another.
        private readonly object wire = new object();
    }
}
