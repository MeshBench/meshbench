//
// The SX1262, as seen from Renode.
//
// There was a model here before: sixteen kilobytes of C# implementing the
// command protocol, the status byte, IRQ flags and the buffer. It worked, and
// it was the wrong shape, because the QEMU backend needed the same chip and
// already had one. Two implementations that have to agree for ever do not, and
// the first time they drifted every comparison between an ARM node and an
// ESP32 node would have been measuring our own code rather than MeshCore's.
//
// After that it forwarded SPI to a `radioserver` process which owned the one
// model. That was right about the chip and wrong about the arrangement: every
// clocked byte was a socket round trip, DIO1 could only be polled because the
// protocol was request-response, and three processes had to have their clocks
// reconciled by anybody asking what happened when.
//
// So the chip is in here now, as the same MIT library QEMU loads and a native
// node links. This half is what the firmware can see - chip select, clocked
// bytes, BUSY and DIO1. The simulated air is VirtualSX1262Engine, beside it.
//
//   Renode --- calls --->  virtual-sx1262
//     \------------ one socket ----------> the RF engine
//
using System;
using Antmicro.Renode.Core;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals.SPI;

namespace Antmicro.Renode.Peripherals.Radio
{
    public class VirtualSX1262 : ISPIPeripheral, IGPIOReceiver, IDisposable
    {
        public VirtualSX1262(IMachine machine, string engineHost = "127.0.0.1",
                             int enginePort = 0)
        {
            this.machine = machine;
            this.engineHost = engineHost;
            this.enginePort = enginePort;
            // Held in a field because the library keeps the pointer: a delegate
            // that only exists as an argument is collected, and the chip then
            // calls into freed memory the first time a packet arrives.
            dio1Callback = new VsxDio1Callback(OnDio1);
        }

        // DIO1 into the MCU. Wired in the platform description to the pin the
        // board uses - P1.15 on a RAK4631.
        public GPIO IRQ { get; private set; } = new GPIO();

        // Connect when the script says so rather than in the constructor: a
        // platform description is loaded before anything else is ready, and a
        // peripheral that throws while the machine is being built takes the
        // whole machine with it.
        public void Connect()
        {
            // Idempotent, because a script can say Connect twice and the second
            // one must not take the first one's place. It would: the engine
            // allows one radio per node and closes the newcomer, and this would
            // then be holding a closed socket while the reader thread served
            // the live one, so the node would go quiet with its connection
            // still established and nothing anywhere saying why.
            if(chip != IntPtr.Zero)
            {
                this.Log(LogLevel.Warning, "already connected; ignoring");
                return;
            }
            var path = Environment.GetEnvironmentVariable("MESHBENCH_RADIO_LIB");
            if(string.IsNullOrEmpty(path))
            {
                this.Log(LogLevel.Error, "MESHBENCH_RADIO_LIB is not set, so this " +
                    "node has no chip and will report chip-not-found rather than say so");
                return;
            }
            try
            {
                lib = VirtualSX1262Lib.Open(path);
                chip = lib.Create();
            }
            catch(Exception e)
            {
                // Loud. An unattached radio answers every register read with
                // zero, which RadioLib reports as no chip present: a
                // configuration error dressed as a hardware fault.
                this.Log(LogLevel.Error, "no chip model: {0}", e.Message);
                return;
            }

            // The seed for this node's receiver noise, which is where its
            // firmware gets its entropy: RadioLib reads the chip's
            // instantaneous RSSI for random bits and MeshCore derives its
            // identity from them. Every node needs its own stream, or every
            // node comes up with the same keypair.
            var seed = Environment.GetEnvironmentVariable("MESHBENCH_NOISE_SEED");
            ulong seedValue;
            if(!string.IsNullOrEmpty(seed) && ulong.TryParse(seed, out seedValue))
            {
                lib.SetNoiseSeed(chip, seedValue);
            }
            lib.SetDio1Callback(chip, dio1Callback, IntPtr.Zero);
            this.Log(LogLevel.Info, "chip attached from {0}", path);

            if(enginePort == 0)
            {
                this.Log(LogLevel.Warning, "no engine port given, so this node is " +
                    "deaf and mute: it will boot and then wait for ever on a " +
                    "transmission that cannot complete");
                return;
            }
            engine = new VirtualSX1262Engine(lib, chip, chipLock, SettleIrq,
                                             () => femLevel,
                                             m => this.Log(LogLevel.Warning, m));
            if(engine.Connect(engineHost, enginePort))
            {
                this.Log(LogLevel.Info, "joined the engine at {0}:{1}",
                         engineHost, enginePort);
            }
        }

        // Chip select. RadioLib drives NSS as an ordinary GPIO rather than
        // letting the SPI controller do it, which is why this is an
        // IGPIOReceiver as well: the transaction boundary arrives on a pin, not
        // from the bus. The rising edge is the only thing that says a command is
        // complete, because an SX1262 command carries no length. Getting this
        // wrong does not fail loudly - the chip sees one unframed byte stream
        // and answers nothing that makes sense.
        public void OnGPIO(int number, bool value)
        {
            if(chip == IntPtr.Zero)
            {
                return;
            }
            if(number != NssPin)
            {
                return;
            }
            lock(chipLock)
            {
                if(!value)                      // active low, as on the part
                {
                    lib.SpiBegin(chip);
                    selected = true;
                }
                else if(selected)
                {
                    lib.SpiEnd(chip);
                    selected = false;
                }
            }
            SettleIrq();
        }

        public byte Transmit(byte data)
        {
            byte answer;

            if(chip == IntPtr.Zero)
            {
                return 0;
            }
            lock(chipLock)
            {
                // The controller may clock bytes without ever touching NSS, if
                // the platform wires chip select to the peripheral itself. Open
                // a transaction rather than dropping the byte: a silent zero
                // here is the hardest kind of fault to find.
                if(!selected)
                {
                    lib.SpiBegin(chip);
                    selected = true;
                    implicitSelect = true;
                }
                answer = lib.SpiByte(chip, data);
            }
            SettleIrq();
            return answer;
        }

        public void FinishTransmission()
        {
            if(chip == IntPtr.Zero || !selected || !implicitSelect)
            {
                return;
            }
            lock(chipLock)
            {
                lib.SpiEnd(chip);
                selected = false;
                implicitSelect = false;
            }
            SettleIrq();
        }

        // BUSY, read as a GPIO by RadioLib between commands. The chip answers,
        // rather than this returning a constant, so an ARM node is not a
        // different radio from every other node in the scenario.
        public bool Busy
        {
            get { return chip != IntPtr.Zero && lib.Busy(chip) != 0; }
        }

        public void Reset()
        {
            selected = false;
            implicitSelect = false;
        }

        // Called from inside a chip call, on whichever thread made it, with
        // chipLock held. So it records the level rather than driving the line:
        // setting a GPIO can enter the CPU, and doing that under our lock while
        // the CPU thread waits on the same lock in Transmit is a deadlock.
        private void OnDio1(IntPtr user, int asserted)
        {
            pendingIrq = asserted != 0;
            irqPending = true;
        }

        private void SettleIrq()
        {
            if(!irqPending)
            {
                return;
            }
            irqPending = false;
            var level = pendingIrq;
            if(level != irqLine)
            {
                irqLine = level;
                IRQ.Set(level);
            }
        }

        public void Dispose()
        {
            if(engine != null)
            {
                engine.Dispose();
                engine = null;
            }
            if(chip != IntPtr.Zero)
            {
                lock(chipLock)
                {
                    lib.Destroy(chip);
                    chip = IntPtr.Zero;
                }
            }
        }

        // Which GPIO carries chip select into this peripheral. Renode numbers
        // the connections a platform declares, so this is the index in the
        // .repl rather than a pin on the board.
        //
        // No front-end module line. heltec_t096 is an nRF52 that carries one,
        // and RenodeWiring has no pin for it and no .repl wires it, so this
        // reports the module as never switched in - which is what it did before
        // as well. Wiring it means a pin per board profile, read from each
        // variant, and is worth doing on its own rather than half here.
        private const int NssPin = 0;

        private readonly IMachine machine;
        private readonly string engineHost;
        private readonly int enginePort;
        private readonly VsxDio1Callback dio1Callback;

        // SPI arrives on the CPU thread and the engine on its own, and both
        // reach the same chip. The library says plainly that it is not thread
        // safe and that the host serialises: this is that.
        private readonly object chipLock = new object();

        private VirtualSX1262Lib lib;
        private IntPtr chip = IntPtr.Zero;
        private VirtualSX1262Engine engine;

        private bool selected;
        private bool implicitSelect;
        private readonly bool femLevel;
        private bool irqLine;
        private volatile bool irqPending;
        private volatile bool pendingIrq;
    }
}
