//
// CLOCK — the nRF52840 clock controller, including the calibration timer.
//
// Renode ships an NRF_CLOCK and it is partial. It starts the oscillators, which
// is enough for most firmware, and it has no calibration timer. The SoftDevice
// uses that timer: it calibrates the internal low-frequency RC oscillator on a
// schedule, and on the way through initialisation it stops the timer and waits
// for the stop to be acknowledged:
//
//     bc58: str.w r7, [r4, #0x018]   ; TASKS_CTSTOP
//     bc5c: ldr.w r0, [r4, #0x12c]   ; EVENTS_CTSTOPPED
//     bc60: cmp   r0, #0
//     bc62: beq.n 0xbc5c             ; forever
//
// An event that never arrives, from a task that went nowhere. This replaces
// Renode's model rather than extending it, because a peripheral can only be
// registered once at an address.
//
// Every task completes immediately. Real oscillators take time to settle -
// about 400 us for the crystal - and modelling that would mean a timing model
// that has to agree with the one in internal/rf. It does not: the firmware
// polls these, so completing at once is early rather than wrong, and the RF
// engine owns everything about timing that a result depends on.
//
using Antmicro.Renode.Core;
using Antmicro.Renode.Core.Structure.Registers;
using Antmicro.Renode.Peripherals.Bus;
using Antmicro.Renode.Time;
using Antmicro.Renode.Peripherals.Timers;
using Antmicro.Renode.Time;

namespace Antmicro.Renode.Peripherals.Miscellaneous
{
    public class NRF52840_Clock : BasicDoubleWordPeripheral, IKnownSize
    {
        public NRF52840_Clock(IMachine machine) : base(machine)
        {
            // The calibration timer really does time out, periodically, and
            // that matters more than it looks. POWER_CLOCK is often the only
            // interrupt a SoftDevice leaves enabled while it idles, so CTTO is
            // what brings the CPU back out of WFE. Stubbing the timer to
            // complete instantly and never fire again leaves an idle machine
            // with nothing at all to wake it.
            //
            // CTIV counts quarter-seconds, so the timer runs at 4 Hz and the
            // limit is the interval itself.
            calTimer = new LimitTimer(machine.ClockSource, 4, this, "ctto",
                                      limit: 1, eventEnabled: true,
                                      direction: Direction.Ascending,
                                      enabled: false, workMode: WorkMode.Periodic);
            calTimer.LimitReached += () =>
            {
                calTimerTimeout.Value = true;
                UpdateInterrupt();
            };

            DefineRegisters();
            Reset();
        }

        public long Size => 0x1000;
        public GPIO IRQ { get; } = new GPIO();

        private void DefineRegisters()
        {
            Registers.HfClkStart.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    hfclkRunning = true;
                    hfclkStarted.Value = true;
                    UpdateInterrupt();
                }, name: "TASKS_HFCLKSTART");

            Registers.HfClkStop.Define(this)
                .WithValueField(0, 32, FieldMode.Write,
                                writeCallback: (_, __) => hfclkRunning = false,
                                name: "TASKS_HFCLKSTOP");

            Registers.LfClkStart.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    lfclkRunning = true;
                    lfclkStarted.Value = true;
                    UpdateInterrupt();
                }, name: "TASKS_LFCLKSTART");

            Registers.LfClkStop.Define(this)
                .WithValueField(0, 32, FieldMode.Write,
                                writeCallback: (_, __) => lfclkRunning = false,
                                name: "TASKS_LFCLKSTOP");

            // Calibration of the internal RC oscillator against the crystal.
            Registers.Calibrate.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    calibrationDone.Value = true;
                    UpdateInterrupt();
                }, name: "TASKS_CAL");

            // The calibration timer: the piece Renode's model does not have.
            Registers.CalTimerStart.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    calTimerStarted.Value = true;
                    calTimer.Limit = calTimerInterval == 0 ? 1 : calTimerInterval;
                    calTimer.Enabled = true;
                    UpdateInterrupt();
                }, name: "TASKS_CTSTART");

            Registers.CalTimerStop.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    calTimer.Enabled = false;
                    calTimerStopped.Value = true;
                    UpdateInterrupt();
                }, name: "TASKS_CTSTOP");

            Registers.HfClkStarted.Define(this)
                .WithFlag(0, out hfclkStarted, name: "EVENTS_HFCLKSTARTED")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.LfClkStarted.Define(this)
                .WithFlag(0, out lfclkStarted, name: "EVENTS_LFCLKSTARTED")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.CalibrationDone.Define(this)
                .WithFlag(0, out calibrationDone, name: "EVENTS_DONE")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.CalTimerTimeout.Define(this)
                .WithFlag(0, out calTimerTimeout, name: "EVENTS_CTTO")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.CalTimerStarted.Define(this)
                .WithFlag(0, out calTimerStarted, name: "EVENTS_CTSTARTED")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.CalTimerStopped.Define(this)
                .WithFlag(0, out calTimerStopped, name: "EVENTS_CTSTOPPED")
                .WithWriteCallback((_, __) => UpdateInterrupt());

            Registers.InterruptEnableSet.Define(this)
                .WithValueField(0, 32, FieldMode.Read | FieldMode.Set,
                                writeCallback: (_, value) => interruptEnabled |= (uint)value,
                                valueProviderCallback: _ => interruptEnabled,
                                name: "INTENSET")
                .WithWriteCallback((_, __) => UpdateInterrupt());

            Registers.InterruptEnableClear.Define(this)
                .WithValueField(0, 32, FieldMode.Read | FieldMode.WriteOneToClear,
                                writeCallback: (_, value) => interruptEnabled &= ~(uint)value,
                                valueProviderCallback: _ => interruptEnabled,
                                name: "INTENCLR")
                .WithWriteCallback((_, __) => UpdateInterrupt());

            Registers.HfClkRun.Define(this)
                .WithValueField(0, 32, FieldMode.Read,
                                valueProviderCallback: _ => hfclkRunning ? 1u : 0u,
                                name: "HFCLKRUN");
            // Bit 16 is "running", bit 0 is "the crystal rather than the RC".
            Registers.HfClkStat.Define(this)
                .WithValueField(0, 32, FieldMode.Read,
                                valueProviderCallback: _ => hfclkRunning ? 0x10001u : 0u,
                                name: "HFCLKSTAT");
            Registers.LfClkRun.Define(this)
                .WithValueField(0, 32, FieldMode.Read,
                                valueProviderCallback: _ => lfclkRunning ? 1u : 0u,
                                name: "LFCLKRUN");
            Registers.LfClkStat.Define(this)
                .WithValueField(0, 32, FieldMode.Read,
                                valueProviderCallback: _ => lfclkRunning ? (0x10000u | lfclkSource) : 0u,
                                name: "LFCLKSTAT");
            Registers.LfClkSrcCopy.Define(this)
                .WithValueField(0, 32, FieldMode.Read,
                                valueProviderCallback: _ => lfclkSource,
                                name: "LFCLKSRCCOPY");
            Registers.LfClkSrc.Define(this)
                .WithValueField(0, 32,
                                writeCallback: (_, value) => lfclkSource = (uint)value & 0x3,
                                valueProviderCallback: _ => lfclkSource,
                                name: "LFCLKSRC");
            Registers.CalTimerInterval.Define(this)
                .WithValueField(0, 32,
                                writeCallback: (_, value) => calTimerInterval = (uint)value,
                                valueProviderCallback: _ => calTimerInterval,
                                name: "CTIV");
        }

        private void UpdateInterrupt()
        {
            var pending =
                (hfclkStarted.Value ? HfClkStartedBit : 0u) |
                (lfclkStarted.Value ? LfClkStartedBit : 0u) |
                (calibrationDone.Value ? DoneBit : 0u) |
                (calTimerTimeout.Value ? CtToBit : 0u) |
                (calTimerStarted.Value ? CtStartedBit : 0u) |
                (calTimerStopped.Value ? CtStoppedBit : 0u);
            IRQ.Set((pending & interruptEnabled) != 0);
        }

        private const uint HfClkStartedBit = 1u << 0;
        private const uint LfClkStartedBit = 1u << 1;
        private const uint DoneBit = 1u << 3;
        private const uint CtToBit = 1u << 4;
        private const uint CtStartedBit = 1u << 10;
        private const uint CtStoppedBit = 1u << 11;

        private IFlagRegisterField hfclkStarted, lfclkStarted, calibrationDone;
        private IFlagRegisterField calTimerTimeout, calTimerStarted, calTimerStopped;
        private bool hfclkRunning, lfclkRunning;
        private readonly LimitTimer calTimer;
        private uint interruptEnabled, lfclkSource, calTimerInterval;

        private enum Registers : long
        {
            HfClkStart = 0x000,
            HfClkStop = 0x004,
            LfClkStart = 0x008,
            LfClkStop = 0x00C,
            Calibrate = 0x010,
            CalTimerStart = 0x014,
            CalTimerStop = 0x018,
            HfClkStarted = 0x100,
            LfClkStarted = 0x104,
            CalibrationDone = 0x10C,
            CalTimerTimeout = 0x110,
            CalTimerStarted = 0x128,
            CalTimerStopped = 0x12C,
            InterruptEnableSet = 0x304,
            InterruptEnableClear = 0x308,
            HfClkRun = 0x408,
            HfClkStat = 0x40C,
            LfClkRun = 0x414,
            LfClkStat = 0x418,
            LfClkSrcCopy = 0x41C,
            LfClkSrc = 0x518,
            CalTimerInterval = 0x538,
        }
    }
}
