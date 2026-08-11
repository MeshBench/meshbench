//
// TEMP — the nRF52840's on-die temperature sensor.
//
// Renode's nRF52840 platform does not model it, and that is where the
// SoftDevice stops. Early in sd_softdevice_enable it starts a temperature
// measurement and waits for the result:
//
//     1604a: wfe
//     1604c: ldr.w r2, [r4, #0x100]   ; EVENTS_DATARDY
//     16050: cmp   r2, #0
//     16052: beq   0x1604a            ; forever
//
// An unmodelled peripheral swallows TASKS_START and never raises DATARDY, so
// the CPU sleeps for ever having executed 28,000 instructions. From outside
// that looks like a proprietary binary refusing to run, which is what it was
// mistaken for. It is a missing four-register peripheral.
//
// The SoftDevice uses the reading to compensate the crystal oscillator, so the
// value only has to be plausible; what matters is that the event arrives.
//
using Antmicro.Renode.Core;
using Antmicro.Renode.Core.Structure.Registers;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals.Bus;

namespace Antmicro.Renode.Peripherals.Sensors
{
    public class NRF52840_Temp : BasicDoubleWordPeripheral, IKnownSize
    {
        public NRF52840_Temp(IMachine machine) : base(machine)
        {
            DefineRegisters();
            Reset();
        }

        public long Size => 0x1000;

        // Degrees Celsius. The register is in quarter-degrees, signed, so 25 C
        // is 100. Settable so a scenario can ask what a node does when it is
        // cold, which is a real question for a mast in February.
        public int TemperatureC { get; set; } = 25;

        private void DefineRegisters()
        {
            Registers.Start.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    // Real silicon takes about 36 us. Completing immediately is
                    // the same lie SimHal tells about BUSY, and for the same
                    // reason: the alternative is a second timing model that has
                    // to agree with the first.
                    dataReady.Value = true;
                    UpdateInterrupt();
                }, name: "TASKS_START");

            Registers.Stop.Define(this)
                .WithValueField(0, 32, FieldMode.Write, name: "TASKS_STOP");

            Registers.DataReady.Define(this)
                .WithFlag(0, out dataReady, name: "EVENTS_DATARDY")
                .WithWriteCallback((_, __) => UpdateInterrupt());

            Registers.InterruptEnableSet.Define(this)
                .WithFlag(0, out interruptEnabled, FieldMode.Read | FieldMode.Set,
                          name: "INTENSET_DATARDY")
                .WithWriteCallback((_, __) => UpdateInterrupt());

            Registers.InterruptEnableClear.Define(this)
                .WithFlag(0, FieldMode.Read | FieldMode.WriteOneToClear,
                          writeCallback: (_, value) => { if(value) interruptEnabled.Value = false; },
                          valueProviderCallback: _ => interruptEnabled.Value,
                          name: "INTENCLR_DATARDY")
                .WithWriteCallback((_, __) => UpdateInterrupt());

            Registers.Temperature.Define(this)
                .WithValueField(0, 32, FieldMode.Read,
                                valueProviderCallback: _ => (ulong)(TemperatureC * 4),
                                name: "TEMP");
        }

        private void UpdateInterrupt()
        {
            IRQ.Set(dataReady.Value && interruptEnabled.Value);
        }

        public GPIO IRQ { get; } = new GPIO();

        private IFlagRegisterField dataReady;
        private IFlagRegisterField interruptEnabled;

        private enum Registers : long
        {
            Start = 0x000,
            Stop = 0x004,
            DataReady = 0x100,
            InterruptEnableSet = 0x304,
            InterruptEnableClear = 0x308,
            Temperature = 0x508,
        }
    }
}
