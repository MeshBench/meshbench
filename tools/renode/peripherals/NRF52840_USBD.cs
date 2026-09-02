//
// USBD, the nRF52840's USB device controller, and the register half of the
// console a MeshCore nRF52 board actually prints on.
//
// The Adafruit nRF52 core builds with USE_TINYUSB, and Adafruit_USBD_CDC.h does
// "#define SerialTinyUSB Serial", so the firmware's Serial is a USB CDC device
// and not UART0. Traced over a live boot, a published RAK4631 image touches
// UART0 once, for 150 ms, to look for a GPS on P0.16/P0.15 and then lets it go.
// Renode's nRF52840 platform has no USBD at all, so everything the workbench
// drives by console - the console pane, provisioning, fleet commands - had
// nothing to reach on these boards.
//
// What this models, and what it does not:
//
//   modelled  EasyDMA on the control and bulk endpoints, the event and
//             interrupt block, EP0 SETUP latching, hardware-handled
//             set-address, EPDATASTATUS, and the frame tick.
//   not       isochronous endpoints, SUSPEND/RESUME and remote wakeup, STALL
//             recovery, DTOGGLE, low power, and every error the real bus can
//             raise. A write to any of those is accepted and logged, never
//             silently dropped.
//
// The distinction is the lesson from the UART: Renode's NRF52840_UART
// implements the EasyDMA half and logs a legacy TXD write as unhandled, so the
// firmware printed into the emulator's log and looked mute. A peripheral that
// is present but partial is harder to spot than one that is absent, so the gaps
// here are named rather than left to be discovered.
//
// The other half of a USB link is a host, and there is no host in an emulator.
// UsbCdcHost is the smallest one that gets TinyUSB to its configured state; it
// drives this model from the outside exactly as a real host would, over the
// frame tick. Where each register sits, and the two traps in that, are in
// UsbdRegisters.cs. VBUS comes from the POWER half of NRF52840_Clock.cs, which
// shares this part's first page.
//
using System;
using System.Collections.Generic;
using Antmicro.Renode.Core;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals.Bus;
using Antmicro.Renode.Peripherals.Timers;
using Antmicro.Renode.Peripherals.UART;
using Antmicro.Renode.Time;

namespace Antmicro.Renode.Peripherals.USB
{
    public class NRF52840_USBD : IDoubleWordPeripheral, IUART, IUsbCdcDevice, IKnownSize
    {
        public NRF52840_USBD(IMachine machine)
        {
            this.machine = machine;
            host = new UsbCdcHost(this);
            // A kilohertz is the real bus's frame rate, and it is also the rate
            // the host state machine steps at: enumeration is a dozen frames,
            // which is milliseconds of the board's own time.
            frames = new LimitTimer(machine.ClockSource, 1000, this, "usbframe",
                                    limit: 1, eventEnabled: true,
                                    direction: Direction.Ascending,
                                    enabled: false, workMode: WorkMode.Periodic);
            frames.LimitReached += Frame;
            Reset();
        }

        public long Size => 0x1000;
        public GPIO IRQ { get; } = new GPIO();

        // IUART. The console is CDC data on the bulk endpoints, so the baud rate
        // the firmware asked for is a fiction on both sides of the wire; it is
        // reported because the interface has to answer, not because anything
        // times a bit with it.
        public uint BaudRate => 115200;
        public Bits StopBits => Bits.One;
        public Parity ParityBit => Parity.None;
        public event Action<byte> CharReceived;

        public void WriteChar(byte value)
        {
            lock(typed)
            {
                typed.Enqueue(value);
            }
        }

        public void Reset()
        {
            enabled = pullup = false;
            inten = eventCause = epDataStatus = usbAddr = 0;
            epInEnabled = epOutEnabled = 0;
            events = 0;
            ep0RcvOutArmed = false;
            autoCompleteSetup = false;
            Array.Clear(setup, 0, setup.Length);
            epIn.Clear();
            epOut.Clear();
            Array.Clear(outSize, 0, outSize.Length);
            Array.Clear(outPending, 0, outPending.Length);
            lock(typed)
            {
                typed.Clear();
            }
            host?.Reset();
            frames.Enabled = false;
            IRQ.Unset();
        }

        public uint ReadDoubleWord(long offset)
        {
            if(offset >= Usbd.EventsBase && offset < Usbd.EventsBase + 4 * Usbd.EventCount)
            {
                return ((events >> (int)((offset - Usbd.EventsBase) / 4)) & 1);
            }
            if(offset >= Usbd.SetupBase && offset < Usbd.SetupBase + 4 * setup.Length)
            {
                return setup[(offset - Usbd.SetupBase) / 4];
            }
            if(offset >= Usbd.SizeEpOut && offset < Usbd.SizeEpOut + 4 * Usbd.Endpoints)
            {
                return outSize[(offset - Usbd.SizeEpOut) / 4];
            }
            uint dma;
            if(epIn.TryRead(offset, out dma) || epOut.TryRead(offset, out dma))
            {
                return dma;
            }
            switch(offset)
            {
                case Usbd.InterruptEnable:
                case Usbd.InterruptEnableSet:
                case Usbd.InterruptEnableClear: return inten;
                case Usbd.EventCause: return eventCause;
                case Usbd.EpDataStatus: return epDataStatus;
                case Usbd.UsbAddress: return usbAddr;
                case Usbd.Enable: return enabled ? 1u : 0u;
                case Usbd.PullUp: return pullup ? 1u : 0u;
                case Usbd.EpInEnable: return epInEnabled;
                case Usbd.EpOutEnable: return epOutEnabled;
                case Usbd.FrameCounter: return frameCounter;
            }
            this.Log(LogLevel.Debug, "read from an unmodelled USBD register 0x{0:X3}", offset);
            return 0;
        }

        public void WriteDoubleWord(long offset, uint value)
        {
            // A task fires on a one and not on a write: the driver's bus reset
            // writes a zero to every endpoint's start task, and a model that
            // took those as transfers would begin eight of them on a bus that
            // had just been reset.
            if(offset >= Usbd.StartEpIn && offset < Usbd.StartEpIn + 4 * Usbd.Endpoints)
            {
                if(value != 0)
                {
                    StartIn((int)((offset - Usbd.StartEpIn) / 4));
                }
                return;
            }
            if(offset >= Usbd.StartEpOut && offset < Usbd.StartEpOut + 4 * Usbd.Endpoints)
            {
                if(value != 0)
                {
                    StartOut((int)((offset - Usbd.StartEpOut) / 4));
                }
                return;
            }
            if(offset >= Usbd.EventsBase && offset < Usbd.EventsBase + 4 * Usbd.EventCount)
            {
                var bit = (int)((offset - Usbd.EventsBase) / 4);
                events = value != 0 ? events | (1u << bit) : events & ~(1u << bit);
                UpdateInterrupt();
                return;
            }
            if(offset >= Usbd.SizeEpOut && offset < Usbd.SizeEpOut + 4 * Usbd.Endpoints)
            {
                // The driver writes this to say it will accept a packet; the
                // count itself is the model's to report.
                return;
            }
            if(epIn.TryWrite(offset, value) || epOut.TryWrite(offset, value))
            {
                return;
            }
            WriteControl(offset, value);
        }

        private void WriteControl(long offset, uint value)
        {
            switch(offset)
            {
                case Usbd.Ep0RcvOut: if(value != 0) { ArmEp0Out(); } return;
                case Usbd.Ep0Status: if(value != 0) { host.ControlDone(); } return;
                case Usbd.Ep0Stall: if(value != 0) { host.ControlStalled(); } return;
                case Usbd.InterruptEnable: inten = value; break;
                case Usbd.InterruptEnableSet: inten |= value; break;
                case Usbd.InterruptEnableClear: inten &= ~value; break;
                case Usbd.EventCause: eventCause &= ~value; break;
                case Usbd.EpDataStatus: epDataStatus &= ~value; break;
                case Usbd.EpInEnable: epInEnabled = value; return;
                case Usbd.EpOutEnable: epOutEnabled = value; return;
                case Usbd.Enable: SetEnabled(value != 0); return;
                case Usbd.PullUp: pullup = value != 0; return;
                default:
                    // Isochronous, stall, toggle and low power, plus the two
                    // undocumented addresses errata 166 asks the driver to
                    // write. Accepted so the firmware carries on, and named so
                    // a stall here is not mistaken for a modelled path.
                    this.Log(LogLevel.Debug,
                             "write of 0x{0:X} to an unmodelled USBD register 0x{1:X3}", value, offset);
                    return;
            }
            UpdateInterrupt();
        }

        // The host side, which is what the rest of this peripheral exists to
        // serve. Attached is the one thing the host cannot see for itself: a
        // device only appears on the bus once it has pulled D+ up.
        public bool Attached => enabled && pullup;

        public void HostReset()
        {
            epDataStatus = 0;
            Array.Clear(outPending, 0, outPending.Length);
            Array.Clear(outSize, 0, outSize.Length);
            ep0RcvOutArmed = false;
            // A board whose console stays quiet is the failure this whole model
            // exists to avoid, and from outside it looks the same whether the
            // bus never came up or the firmware never wrote anything. These
            // lines are how the two are told apart; "logLevel 0 sysbus.usbd"
            // turns them on without drowning the run.
            this.Log(LogLevel.Debug, "bus reset, interrupts enabled 0x{0:X}", inten);
            RaiseEvent(Usbd.EvUsbReset);
        }

        // A SETUP packet, latched into the registers the driver reads. Set
        // address is the one the controller answers by itself on real silicon,
        // which is why the driver deliberately does not tell its stack: with
        // nobody to run the status stage, the model has to finish it.
        public void HostSetup(byte[] packet)
        {
            for(var i = 0; i < setup.Length; i++)
            {
                setup[i] = packet[i];
            }
            autoCompleteSetup = packet[0] == 0x00 && packet[1] == SetAddressRequest;
            if(autoCompleteSetup)
            {
                usbAddr = packet[2];
            }
            ep0RcvOutArmed = false;
            this.Log(LogLevel.Debug,
                     "setup {0:X2} {1:X2}, interrupts enabled 0x{2:X}", packet[0], packet[1], inten);
            RaiseEvent(Usbd.EvEp0Setup);
        }

        // Data the host is sending in the OUT stage of a control transfer. It
        // waits for the driver's EP0RCVOUT, because a packet the device has not
        // asked for is one it would not have acknowledged.
        public void HostControlOut(byte[] data)
        {
            outPending[0] = data;
            if(ep0RcvOutArmed)
            {
                ArmEp0Out();
            }
        }

        public bool OutBusy(int ep) => outPending[ep] != null;

        public void HostOut(int ep, byte[] data)
        {
            outPending[ep] = data;
            outSize[ep] = (uint)data.Length;
            epDataStatus |= 1u << (16 + ep);
            RaiseEvent(Usbd.EvEpData);
        }

        public void ToTerminal(byte[] data)
        {
            var sink = CharReceived;
            if(sink == null)
            {
                return;
            }
            foreach(var b in data)
            {
                sink(b);
            }
        }

        private void SetEnabled(bool on)
        {
            enabled = on;
            frames.Enabled = on;
            if(on)
            {
                // The driver blocks on READY before it will touch anything
                // else, and this model has no oscillator to wait for.
                eventCause |= Usbd.EventCauseReady;
            }
        }

        private void ArmEp0Out()
        {
            ep0RcvOutArmed = true;
            if(outPending[0] == null)
            {
                return;
            }
            outSize[0] = (uint)outPending[0].Length;
            RaiseEvent(Usbd.EvEp0DataDone);
        }

        private void StartIn(int ep)
        {
            var length = epIn.Count(ep);
            var data = length > 0
                ? machine.GetSystemBus(this).ReadBytes(epIn.Pointer(ep), length)
                : Empty;
            epIn.Took(ep, data.Length);
            this.Log(LogLevel.Debug, "endpoint {0} in, {1} bytes", ep, data.Length);
            RaiseEvent(Usbd.EvStarted);
            RaiseEvent(Usbd.EvEndEpIn + ep);
            if(ep == 0)
            {
                host.ControlIn(data);
                RaiseEvent(Usbd.EvEp0DataDone);
                return;
            }
            host.BulkIn(ep, data);
            epDataStatus |= 1u << ep;
            RaiseEvent(Usbd.EvEpData);
        }

        private void StartOut(int ep)
        {
            var data = outPending[ep] ?? Empty;
            var length = Math.Min(data.Length, epOut.Count(ep));
            if(length > 0)
            {
                var slice = new byte[length];
                Array.Copy(data, slice, length);
                machine.GetSystemBus(this).WriteBytes(slice, epOut.Pointer(ep));
            }
            epOut.Took(ep, length);
            outPending[ep] = null;
            this.Log(LogLevel.Debug, "endpoint {0} out, {1} bytes", ep, length);
            RaiseEvent(Usbd.EvStarted);
            RaiseEvent(Usbd.EvEndEpOut + ep);
        }

        private void Frame()
        {
            frameCounter = (frameCounter + 1) & 0x7FF;
            if(autoCompleteSetup)
            {
                autoCompleteSetup = false;
                host.ControlDone();
            }
            lock(typed)
            {
                host.Frame(typed);
            }
            // Only when somebody is listening: an event left standing from
            // before the driver enabled the interrupt is one it reads as a
            // frame that has already happened.
            if((inten & (1u << Usbd.EvSof)) != 0)
            {
                RaiseEvent(Usbd.EvSof);
            }
        }

        private void RaiseEvent(int bit)
        {
            events |= 1u << bit;
            UpdateInterrupt();
        }

        private void UpdateInterrupt() => IRQ.Set((events & inten) != 0);

        private const byte SetAddressRequest = 5;

        private static readonly byte[] Empty = new byte[0];

        private readonly IMachine machine;
        private readonly UsbCdcHost host;
        private readonly LimitTimer frames;
        private readonly Queue<byte> typed = new Queue<byte>();

        private readonly UsbdEndpointSlots epIn = new UsbdEndpointSlots(Usbd.EpInBase);
        private readonly UsbdEndpointSlots epOut = new UsbdEndpointSlots(Usbd.EpOutBase);
        private readonly uint[] setup = new uint[8];
        private readonly uint[] outSize = new uint[Usbd.Endpoints];
        private readonly byte[][] outPending = new byte[Usbd.Endpoints][];

        private bool enabled, pullup, ep0RcvOutArmed, autoCompleteSetup;
        private uint inten, events, eventCause, epDataStatus, usbAddr;
        private uint epInEnabled, epOutEnabled, frameCounter;
    }
}
