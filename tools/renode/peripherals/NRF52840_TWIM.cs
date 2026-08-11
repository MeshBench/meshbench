//
// TWIM — the nRF52840's I2C controller, the EasyDMA one.
//
// Renode has an NRF52840_I2C and it models the legacy TWI registers. The part
// the firmware drives is TWIM, which starts a transfer with TASKS_STARTTX and
// reports progress through EVENTS_TXSTARTED, EVENTS_ERROR and EVENTS_STOPPED.
// None of those arrive from the legacy model, so MeshCore's published nRF52
// build spins here for ever while probing for sensors:
//
//     3c1ce: str   r6, [r3, #8]        ; TASKS_STARTTX
//     3c1d4: ldr.w r2, [r3, #0x124]    ; EVENTS_ERROR
//     3c1da: ldr.w r2, [r3, #0x150]    ; EVENTS_TXSTARTED
//     3c1e0: beq.n 0x3c1d4
//
// What this models is a bus with nothing on it, which is the truth: MeshBench
// simulates radio, not sensors. A transfer starts and then the address goes
// unanswered, so ERROR is raised with the address-NACK bit. Firmware probing
// for a sensor gets the answer real hardware would give on a board without one
// - not there - and carries on, rather than waiting for a reply that cannot come.
//
using Antmicro.Renode.Core;
using Antmicro.Renode.Core.Structure.Registers;
using Antmicro.Renode.Peripherals.Bus;

namespace Antmicro.Renode.Peripherals.I2C
{
    public class NRF52840_TWIM : BasicDoubleWordPeripheral, IKnownSize
    {
        public NRF52840_TWIM(IMachine machine) : base(machine)
        {
            DefineRegisters();
            Reset();
        }

        public long Size => 0x1000;
        public GPIO IRQ { get; } = new GPIO();

        private void DefineRegisters()
        {
            Registers.StartRx.Define(this)
                .WithValueField(0, 32, FieldMode.Write,
                                writeCallback: (_, __) => Transfer(receiving: true),
                                name: "TASKS_STARTRX");
            Registers.StartTx.Define(this)
                .WithValueField(0, 32, FieldMode.Write,
                                writeCallback: (_, __) => Transfer(receiving: false),
                                name: "TASKS_STARTTX");
            Registers.Stop.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    stopped.Value = true;
                    UpdateInterrupt();
                }, name: "TASKS_STOP");
            Registers.Suspend.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    suspended.Value = true;
                    UpdateInterrupt();
                }, name: "TASKS_SUSPEND");
            Registers.Resume.Define(this)
                .WithValueField(0, 32, FieldMode.Write, name: "TASKS_RESUME");

            Registers.Stopped.Define(this).WithFlag(0, out stopped, name: "EVENTS_STOPPED")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.Error.Define(this).WithFlag(0, out error, name: "EVENTS_ERROR")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.Suspended.Define(this).WithFlag(0, out suspended, name: "EVENTS_SUSPENDED")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.RxStarted.Define(this).WithFlag(0, out rxStarted, name: "EVENTS_RXSTARTED")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.TxStarted.Define(this).WithFlag(0, out txStarted, name: "EVENTS_TXSTARTED")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.LastRx.Define(this).WithFlag(0, out lastRx, name: "EVENTS_LASTRX")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.LastTx.Define(this).WithFlag(0, out lastTx, name: "EVENTS_LASTTX")
                .WithWriteCallback((_, __) => UpdateInterrupt());

            // ERRORSRC: bit 1 is address NACK, which is what an empty bus gives.
            Registers.ErrorSource.Define(this)
                .WithValueField(0, 32, FieldMode.Read | FieldMode.WriteOneToClear,
                                valueProviderCallback: _ => AddressNack, name: "ERRORSRC");

            Registers.InterruptEnableSet.Define(this)
                .WithValueField(0, 32, FieldMode.Read | FieldMode.Set,
                                writeCallback: (_, v) => interruptEnabled |= (uint)v,
                                valueProviderCallback: _ => interruptEnabled, name: "INTENSET")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.InterruptEnableClear.Define(this)
                .WithValueField(0, 32, FieldMode.Read | FieldMode.WriteOneToClear,
                                writeCallback: (_, v) => interruptEnabled &= ~(uint)v,
                                valueProviderCallback: _ => interruptEnabled, name: "INTENCLR")
                .WithWriteCallback((_, __) => UpdateInterrupt());

            Registers.Enable.Define(this)
                .WithValueField(0, 32, writeCallback: (_, v) => enabled = v != 0,
                                valueProviderCallback: _ => enabled ? 6u : 0u, name: "ENABLE");
        }

        // Start, then fail to find anybody. Both events, in that order, because
        // firmware polls TXSTARTED to know the controller took the job and ERROR
        // to know how it ended.
        private void Transfer(bool receiving)
        {
            if(receiving)
            {
                rxStarted.Value = true;
                lastRx.Value = true;
            }
            else
            {
                txStarted.Value = true;
                lastTx.Value = true;
            }
            error.Value = true;
            stopped.Value = true;
            UpdateInterrupt();
        }

        private void UpdateInterrupt()
        {
            var pending =
                (stopped.Value ? 1u << 1 : 0) |
                (error.Value ? 1u << 9 : 0) |
                (suspended.Value ? 1u << 18 : 0) |
                (rxStarted.Value ? 1u << 19 : 0) |
                (txStarted.Value ? 1u << 20 : 0) |
                (lastRx.Value ? 1u << 23 : 0) |
                (lastTx.Value ? 1u << 24 : 0);
            IRQ.Set((pending & interruptEnabled) != 0);
        }

        private const uint AddressNack = 1u << 1;

        private IFlagRegisterField stopped, error, suspended, rxStarted, txStarted, lastRx, lastTx;
        private uint interruptEnabled;
        private bool enabled;

        private enum Registers : long
        {
            StartRx = 0x000,
            StartTx = 0x008,
            Stop = 0x014,
            Suspend = 0x01C,
            Resume = 0x020,
            Stopped = 0x104,
            Error = 0x124,
            Suspended = 0x148,
            RxStarted = 0x14C,
            TxStarted = 0x150,
            LastRx = 0x15C,
            LastTx = 0x160,
            InterruptEnableSet = 0x304,
            InterruptEnableClear = 0x308,
            ErrorSource = 0x4C4,
            Enable = 0x500,
        }
    }
}
