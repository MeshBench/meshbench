//
// The line from an emulated node's radio to the RF engine.
//
// Separate from the peripheral because it is a different job. The peripheral is
// pins and SPI, which is what the firmware can see; this is the simulated air,
// which it cannot: what else is transmitting, when a waveform this node started
// has finished occupying the channel, and what arrived. Only the engine knows
// any of that, because the channel is shared with every other node in the
// scenario.
//
// The chip is not here. It belongs to the peripheral, and this reaches it under
// the lock the peripheral owns, because SPI arrives on the CPU thread and this
// runs on its own. The library says plainly that it is not thread safe and that
// the host serialises; that lock is the host doing so.
//
// Wire format, both directions: [kind:1][length:2 big-endian][payload]. Shared
// with the native firmware bridge and with the simulator's Go half, and frozen
// by both.
//
using System;
using System.IO;
using System.Net.Sockets;
using System.Threading;

namespace Antmicro.Renode.Peripherals.Radio
{
    public class VirtualSX1262Engine : IDisposable
    {
        // settle is called after anything that could have moved the DIO1 line,
        // outside the chip lock. It is a callback rather than a direct GPIO
        // write because setting a Renode GPIO can enter the CPU, and doing that
        // under the lock while the CPU thread waits on the same lock in
        // Transmit is a deadlock.
        public VirtualSX1262Engine(VirtualSX1262Lib lib, IntPtr chip, object chipLock,
                                   Action settle, Func<bool> femLevel,
                                   Action<string> log)
        {
            this.lib = lib;
            this.chip = chip;
            this.chipLock = chipLock;
            this.settle = settle;
            this.femLevel = femLevel;
            this.log = log;
        }

        // Returns false when there is no engine to join, which is a node that
        // will boot and then wait for ever on a transmission that cannot
        // complete. The caller says so; this only reports.
        public bool Connect(string host, int port)
        {
            try
            {
                client = new TcpClient(host, port) { NoDelay = true };
                stream = client.GetStream();
            }
            catch(Exception e)
            {
                log(string.Format("no engine at {0}:{1} - {2}", host, port, e.Message));
                return false;
            }
            // The stream is handed to the thread rather than read from the
            // field on every pass: this thread must serve the connection it was
            // started for and no other, whatever else happens to the field.
            thread = new Thread(Serve)
            {
                IsBackground = true,
                Name = "virtual-sx1262 engine"
            };
            thread.Start(stream);
            return true;
        }

        private void Serve(object opaque)
        {
            var wire = (NetworkStream)opaque;
            var header = new byte[3];

            while(true)
            {
                try
                {
                    if(!ReadAll(wire, header, 3))
                    {
                        break;
                    }
                    var length = (header[1] << 8) | header[2];
                    var payload = new byte[length];
                    if(length > 0 && !ReadAll(wire, payload, length))
                    {
                        break;
                    }
                    Handle(header[0], payload);
                }
                catch(Exception e)
                {
                    log("the engine went away - " + e.Message);
                    break;
                }
            }
            stream = null;
        }

        private void Handle(byte kind, byte[] payload)
        {
            switch(kind)
            {
            case Frame:
                // A packet the channel delivered. Only frames that passed CRC
                // arrive here, exactly as on hardware: everything else was
                // recorded and withheld.
                if(payload.Length > 0)
                {
                    lock(chipLock)
                    {
                        lib.DeliverFrame(chip, payload, (UIntPtr)payload.Length);
                    }
                }
                break;

            case TxDone:
                lock(chipLock)
                {
                    lib.TransmitFinished(chip);
                }
                break;

            case ChannelBusy:
                if(payload.Length >= 1)
                {
                    lock(chipLock)
                    {
                        lib.SetChannelBusy(chip, payload[0] != 0 ? 1 : 0);
                    }
                }
                break;

            case Tick:
                if(payload.Length == 4)
                {
                    Advance(((uint)payload[0] << 24) | ((uint)payload[1] << 16) |
                            ((uint)payload[2] << 8) | payload[3]);
                }
                break;

            default:
                // Skipped, not fatal. Console traffic reaches an emulated node
                // over the emulator's own serial port, so the engine's console
                // messages arrive here and are meant to be ignored. Treating an
                // unknown kind as fatal once killed the radio the moment
                // anybody typed at the fleet, and the node then reported "radio
                // init failed: -2", which points at wiring.
                break;
            }
            settle();
        }

        private void Advance(uint toMs)
        {
            lock(chipLock)
            {
                // A millisecond at a time, as a native node is stepped.
                // Stepping rather than jumping is what keeps the chip's own
                // timeouts behaving: a preamble flag that should clear after
                // 66 ms does not, if time arrives in 500 ms lumps.
                while(simMs < toMs)
                {
                    simMs++;
                    lib.Tick(chip, simMs);
                    DrainTx();
                }
                lib.Tick(chip, simMs);
                DrainTx();
                SendStats();
            }
            settle();

            var ack = new byte[4];
            Put32(ack, 0, toMs);
            Send(Ack, ack, ack.Length);
        }

        // Anything the firmware handed its radio goes out to the engine now.
        //
        // A transmission reaches the channel immediately and is not immediately
        // complete: the chip stays in transmit until the engine sends TxDone,
        // exactly as a native node does, because that is what stops a node
        // talking over itself.
        //
        // Called with chipLock held.
        private void DrainTx()
        {
            var n = (int)lib.TakeTx(chip, txBuffer, (UIntPtr)txBuffer.Length);
            if(n <= 0)
            {
                return;
            }
            if(n > txBuffer.Length)
            {
                // Truncated, and said out loud. A frame this long is not
                // something MeshCore sends, so it means the chip and this
                // disagree about the buffer rather than that a node had a lot
                // to say.
                log(string.Format("the chip offered a {0} byte frame", n));
                n = txBuffer.Length;
            }
            Send(Frame, txBuffer, n);
        }

        // What this radio has been configured to be, and what it has counted.
        //
        // The native bridge and QEMU's device write the same payload in the
        // same order: an emulated node and a native one reporting different
        // shapes would make every comparison between them a comparison of our
        // own code. Called with chipLock held.
        private void SendStats()
        {
            VsxState state;
            VsxCounters counters;
            var sb = new byte[StatsLength];

            lib.GetState(chip, out state);
            lib.GetCounters(chip, out counters);

            Put32(sb, 0, counters.IrqReads);
            Put32(sb, 4, counters.BusyReads);
            Put32(sb, 8, counters.BusyMs);
            Put32(sb, 12, counters.SpuriousRaises);

            sb[16] = state.RxGainReg;
            sb[17] = (byte)state.TxPowerDbm;
            // The line as it stands now, which the peripheral knows because the
            // board drives it there: the module sits beside the chip, not
            // inside it, so the chip has no view of it and reports none.
            sb[18] = (byte)(femLevel() ? 1 : 0);
            sb[19] = state.Mode;
            sb[20] = state.SpreadingFactor;
            sb[21] = state.CodingRate;
            Put32(sb, 22, state.FreqHz);
            Put32(sb, 26, state.BandwidthHz);
            Put16(sb, 30, state.PreambleSyms);
            Put16(sb, 32, state.IrqMask);
            Put16(sb, 34, state.IrqFlags);
            // Three states, because "has not transmitted" is not "transmitted
            // with the module out".
            sb[36] = state.FemAtTx;
            // The DIO1 routing mask, which is not the enable mask above.
            // Reported separately because confusing the two is a fault that has
            // already happened: HeaderValid raised DIO1 part-way through a
            // carrier, the pin was still high when RxDone arrived, and a driver
            // that attaches on the rising edge never learned the packet existed.
            Put16(sb, 37, state.Dio1Mask);

            Send(RadioStats, sb, sb.Length);
        }

        private static bool ReadAll(NetworkStream wire, byte[] buffer, int count)
        {
            var at = 0;
            while(at < count)
            {
                var got = wire.Read(buffer, at, count - at);
                if(got <= 0)
                {
                    return false;
                }
                at += got;
            }
            return true;
        }

        private void Send(byte kind, byte[] payload, int length)
        {
            var wire = stream;
            if(wire == null)
            {
                return;
            }
            try
            {
                lock(writeLock)
                {
                    wire.WriteByte(kind);
                    wire.WriteByte((byte)(length >> 8));
                    wire.WriteByte((byte)length);
                    if(length > 0)
                    {
                        wire.Write(payload, 0, length);
                    }
                }
            }
            catch(IOException e)
            {
                log("the engine stopped listening - " + e.Message);
                stream = null;
            }
        }

        private static void Put32(byte[] b, int at, uint v)
        {
            b[at] = (byte)(v >> 24);
            b[at + 1] = (byte)(v >> 16);
            b[at + 2] = (byte)(v >> 8);
            b[at + 3] = (byte)v;
        }

        private static void Put16(byte[] b, int at, ushort v)
        {
            b[at] = (byte)(v >> 8);
            b[at + 1] = (byte)v;
        }

        public void Dispose()
        {
            var wire = stream;
            stream = null;
            if(wire != null)
            {
                wire.Close();
            }
            client = null;
        }

        private const byte Frame = 0x01;
        private const byte Tick = 0x02;
        private const byte Ack = 0x03;
        private const byte TxDone = 0x04;
        private const byte ChannelBusy = 0x08;
        private const byte RadioStats = 0x09;

        // The stats record, whose layout the engine reads on length.
        private const int StatsLength = 39;

        private readonly VirtualSX1262Lib lib;
        private readonly IntPtr chip;
        private readonly object chipLock;
        private readonly Action settle;
        private readonly Func<bool> femLevel;
        private readonly Action<string> log;
        private readonly byte[] txBuffer = new byte[512];
        private readonly object writeLock = new object();

        // Held only so it is not collected: a TcpClient with no reference left
        // is finalised, and finalising it closes the socket the reader thread
        // is still serving.
        private TcpClient client;
        private NetworkStream stream;
        private Thread thread;
        private uint simMs;
    }
}
