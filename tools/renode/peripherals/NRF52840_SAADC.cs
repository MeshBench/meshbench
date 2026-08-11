//
// SAADC — the nRF52840's analogue-to-digital converter.
//
// Renode's nRF52840 platform does not model it, and MeshCore's published nRF52
// builds read the battery through it during start-up. Without it the firmware
// starts a conversion and spins on EVENTS_STARTED for ever:
//
//     72548: str   r6, [r1, #0]         ; TASKS_START
//     7254a: ldr.w r3, [r0, #0x100]     ; EVENTS_STARTED
//     7254e: cmp   r3, #0
//     72550: beq.n 0x7254a
//
// The conversion result goes to RAM by EasyDMA rather than through a register,
// so a model that only sets the events leaves the firmware reading whatever was
// in the buffer. It writes the samples too.
//
// The battery reading is not a simulated quantity — nothing in MeshBench models
// a battery — so it answers a fixed, plainly full reading. A node that thinks it
// is flat behaves differently, and inventing a discharge curve here would be
// putting made-up physics in the one place nobody would look for it.
//
using Antmicro.Renode.Core;
using Antmicro.Renode.Core.Structure.Registers;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals.Bus;

namespace Antmicro.Renode.Peripherals.Analog
{
    public class NRF52840_SAADC : BasicDoubleWordPeripheral, IKnownSize
    {
        public NRF52840_SAADC(IMachine machine) : base(machine)
        {
            this.machine = machine;
            DefineRegisters();
            Reset();
        }

        public long Size => 0x1000;
        public GPIO IRQ { get; } = new GPIO();

        // What every channel reads, in raw counts. The default corresponds to a
        // healthy battery through the RAK4631's divider; a scenario that wants a
        // flat node can set it.
        public uint Sample { get; set; } = 3000;

        private void DefineRegisters()
        {
            Registers.Start.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    started.Value = true;
                    UpdateInterrupt();
                }, name: "TASKS_START");

            Registers.Sample.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    WriteSamples();
                    end.Value = true;
                    UpdateInterrupt();
                }, name: "TASKS_SAMPLE");

            Registers.Stop.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    stopped.Value = true;
                    UpdateInterrupt();
                }, name: "TASKS_STOP");

            Registers.Calibrate.Define(this)
                .WithValueField(0, 32, FieldMode.Write, writeCallback: (_, __) =>
                {
                    calibrateDone.Value = true;
                    UpdateInterrupt();
                }, name: "TASKS_CALIBRATEOFFSET");

            Registers.Started.Define(this).WithFlag(0, out started, name: "EVENTS_STARTED")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.End.Define(this).WithFlag(0, out end, name: "EVENTS_END")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.Done.Define(this).WithFlag(0, out done, name: "EVENTS_DONE")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.ResultDone.Define(this).WithFlag(0, out resultDone, name: "EVENTS_RESULTDONE")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.CalibrateDone.Define(this).WithFlag(0, out calibrateDone, name: "EVENTS_CALIBRATEDONE")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.Stopped.Define(this).WithFlag(0, out stopped, name: "EVENTS_STOPPED")
                .WithWriteCallback((_, __) => UpdateInterrupt());

            Registers.InterruptEnableSet.Define(this)
                .WithValueField(0, 32, FieldMode.Read | FieldMode.Set,
                                writeCallback: (_, value) => interruptEnabled |= (uint)value,
                                valueProviderCallback: _ => interruptEnabled, name: "INTENSET")
                .WithWriteCallback((_, __) => UpdateInterrupt());
            Registers.InterruptEnableClear.Define(this)
                .WithValueField(0, 32, FieldMode.Read | FieldMode.WriteOneToClear,
                                writeCallback: (_, value) => interruptEnabled &= ~(uint)value,
                                valueProviderCallback: _ => interruptEnabled, name: "INTENCLR")
                .WithWriteCallback((_, __) => UpdateInterrupt());

            Registers.Status.Define(this)
                .WithValueField(0, 32, FieldMode.Read, valueProviderCallback: _ => 0, name: "STATUS");
            Registers.Enable.Define(this)
                .WithValueField(0, 32, writeCallback: (_, v) => enabled = v != 0,
                                valueProviderCallback: _ => enabled ? 1u : 0u, name: "ENABLE");

            Registers.ResultPointer.Define(this)
                .WithValueField(0, 32, writeCallback: (_, v) => resultPointer = (uint)v,
                                valueProviderCallback: _ => resultPointer, name: "RESULT.PTR");
            Registers.ResultMaxCount.Define(this)
                .WithValueField(0, 32, writeCallback: (_, v) => resultMaxCount = (uint)v,
                                valueProviderCallback: _ => resultMaxCount, name: "RESULT.MAXCNT");
            Registers.ResultAmount.Define(this)
                .WithValueField(0, 32, FieldMode.Read,
                                valueProviderCallback: _ => resultAmount, name: "RESULT.AMOUNT");
        }

        // EasyDMA: the samples land in RAM, and the firmware reads them there.
        private void WriteSamples()
        {
            if(resultPointer == 0 || resultMaxCount == 0)
            {
                return;
            }
            for(var i = 0u; i < resultMaxCount; i++)
            {
                machine.SystemBus.WriteWord(resultPointer + i * 2, (ushort)Sample);
            }
            resultAmount = resultMaxCount;
            resultDone.Value = true;
            done.Value = true;
        }

        private void UpdateInterrupt()
        {
            var pending =
                (started.Value ? 1u << 0 : 0) |
                (end.Value ? 1u << 1 : 0) |
                (done.Value ? 1u << 2 : 0) |
                (resultDone.Value ? 1u << 3 : 0) |
                (calibrateDone.Value ? 1u << 4 : 0) |
                (stopped.Value ? 1u << 5 : 0);
            IRQ.Set((pending & interruptEnabled) != 0);
        }

        private IFlagRegisterField started, end, done, resultDone, calibrateDone, stopped;
        private uint interruptEnabled, resultPointer, resultMaxCount, resultAmount;
        private bool enabled;
        private readonly IMachine machine;

        private enum Registers : long
        {
            Start = 0x000,
            Sample = 0x004,
            Stop = 0x008,
            Calibrate = 0x00C,
            Started = 0x100,
            End = 0x104,
            Done = 0x108,
            ResultDone = 0x10C,
            CalibrateDone = 0x110,
            Stopped = 0x114,
            InterruptEnableSet = 0x304,
            InterruptEnableClear = 0x308,
            Status = 0x400,
            Enable = 0x500,
            ResultPointer = 0x62C,
            ResultMaxCount = 0x630,
            ResultAmount = 0x634,
        }
    }
}
