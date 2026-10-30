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
// What this models is a bus with only what the board declares on it. An
// address nobody claims goes unanswered, so ERROR is raised with the
// address-NACK bit: firmware probing for a sensor gets the answer real hardware
// gives on a board without one - not there - and carries on, rather than
// waiting for a reply that cannot come. That stays the default.
//
// A device registered at an address (a display, `oled: ... @ twi0 0x3C`) is
// answered instead: the transfer's bytes move through EasyDMA between memory
// and the device, the AMOUNT registers say how many, and the shortcuts the
// driver set decide what follows. That is the slave registry #602 asked for.
//
using System.Collections.Generic;
using Antmicro.Renode.Core;
using Antmicro.Renode.Core.Structure;
using Antmicro.Renode.Core.Structure.Registers;
using Antmicro.Renode.Peripherals.Bus;

namespace Antmicro.Renode.Peripherals.I2C
{
    public class NRF52840_TWIM : BasicDoubleWordPeripheral, IKnownSize,
        IPeripheralContainer<II2CPeripheral, NumberRegistrationPoint<int>>
    {
        public NRF52840_TWIM(IMachine machine) : base(machine)
        {
            DefineRegisters();
            Reset();
        }

        public void Register(II2CPeripheral peripheral, NumberRegistrationPoint<int> registrationPoint)
        {
            slaves[registrationPoint.Address] = peripheral;
            machine.RegisterAsAChildOf(this, peripheral, registrationPoint);
        }

        public void Unregister(II2CPeripheral peripheral)
        {
            foreach(var at in new List<int>(slaves.Keys))
            {
                if(slaves[at] == peripheral)
                {
                    slaves.Remove(at);
                }
            }
            machine.UnregisterAsAChildOf(this, peripheral);
        }

        public IEnumerable<NumberRegistrationPoint<int>> GetRegistrationPoints(II2CPeripheral peripheral)
        {
            var points = new List<NumberRegistrationPoint<int>>();
            foreach(var kv in slaves)
            {
                if(kv.Value == peripheral)
                {
                    points.Add(new NumberRegistrationPoint<int>(kv.Key));
                }
            }
            return points;
        }

        public IEnumerable<IRegistered<II2CPeripheral, NumberRegistrationPoint<int>>> Children
        {
            get
            {
                var children = new List<IRegistered<II2CPeripheral, NumberRegistrationPoint<int>>>();
                foreach(var kv in slaves)
                {
                    children.Add(Registered.Create(kv.Value, new NumberRegistrationPoint<int>(kv.Key)));
                }
                return children;
            }
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
                    II2CPeripheral slave;
                    if(slaves.TryGetValue((int)address.Value, out slave))
                    {
                        slave.FinishTransmission();
                    }
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

            // ERRORSRC: bit 1 is address NACK, which is what an empty address gives.
            Registers.ErrorSource.Define(this)
                .WithValueField(0, 32, FieldMode.Read | FieldMode.WriteOneToClear,
                                valueProviderCallback: _ => errorSource,
                                writeCallback: (_, v) => errorSource &= ~(uint)v, name: "ERRORSRC");

            Registers.Shorts.Define(this)
                .WithValueField(0, 32, out shorts, name: "SHORTS");
            Registers.Address.Define(this)
                .WithValueField(0, 7, out address, name: "ADDRESS")
                .WithReservedBits(7, 25);
            Registers.RxdPtr.Define(this).WithValueField(0, 32, out rxdPtr, name: "RXD.PTR");
            Registers.RxdMaxCnt.Define(this).WithValueField(0, 16, out rxdMaxCnt, name: "RXD.MAXCNT")
                .WithReservedBits(16, 16);
            Registers.RxdAmount.Define(this).WithValueField(0, 16, FieldMode.Read,
                valueProviderCallback: _ => rxdAmount, name: "RXD.AMOUNT").WithReservedBits(16, 16);
            Registers.TxdPtr.Define(this).WithValueField(0, 32, out txdPtr, name: "TXD.PTR");
            Registers.TxdMaxCnt.Define(this).WithValueField(0, 16, out txdMaxCnt, name: "TXD.MAXCNT")
                .WithReservedBits(16, 16);
            Registers.TxdAmount.Define(this).WithValueField(0, 16, FieldMode.Read,
                valueProviderCallback: _ => txdAmount, name: "TXD.AMOUNT").WithReservedBits(16, 16);

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

        // Start, then either find nobody - the NACK an empty address gives -
        // or move the bytes to or from the device that is there. Both events
        // in that order, because firmware polls STARTED to know the controller
        // took the job and ERROR or STOPPED to know how it ended.
        private void Transfer(bool receiving)
        {
            if(receiving)
            {
                rxStarted.Value = true;
            }
            else
            {
                txStarted.Value = true;
            }
            II2CPeripheral slave;
            if(!slaves.TryGetValue((int)address.Value, out slave))
            {
                if(receiving)
                {
                    lastRx.Value = true;
                }
                else
                {
                    lastTx.Value = true;
                }
                errorSource = AddressNack;
                error.Value = true;
                stopped.Value = true;
                UpdateInterrupt();
                return;
            }
            if(receiving)
            {
                var want = (int)rxdMaxCnt.Value;
                var got = slave.Read(want) ?? new byte[0];
                var n = System.Math.Min(want, got.Length);
                sysbus.WriteBytes(got, rxdPtr.Value, n);
                rxdAmount = (uint)n;
                lastRx.Value = true;
                if((shorts.Value & ShortLastRxStop) != 0)
                {
                    Stop(slave);
                }
                else if((shorts.Value & ShortLastRxStartTx) != 0)
                {
                    Transfer(receiving: false);
                    return;
                }
            }
            else
            {
                var n = (int)txdMaxCnt.Value;
                var bytes = sysbus.ReadBytes(txdPtr.Value, n);
                slave.Write(bytes);
                txdAmount = (uint)n;
                lastTx.Value = true;
                if((shorts.Value & ShortLastTxStop) != 0)
                {
                    Stop(slave);
                }
                else if((shorts.Value & ShortLastTxStartRx) != 0)
                {
                    Transfer(receiving: true);
                    return;
                }
                else if((shorts.Value & ShortLastTxSuspend) != 0)
                {
                    suspended.Value = true;
                }
            }
            UpdateInterrupt();
        }

        private void Stop(II2CPeripheral slave)
        {
            slave.FinishTransmission();
            stopped.Value = true;
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
        private const ulong ShortLastTxStartRx = 1u << 7;
        private const ulong ShortLastTxSuspend = 1u << 8;
        private const ulong ShortLastTxStop = 1u << 9;
        private const ulong ShortLastRxStartTx = 1u << 10;
        private const ulong ShortLastRxStop = 1u << 12;

        private readonly Dictionary<int, II2CPeripheral> slaves = new Dictionary<int, II2CPeripheral>();
        private IValueRegisterField shorts, address, rxdPtr, rxdMaxCnt, txdPtr, txdMaxCnt;
        private uint rxdAmount, txdAmount, errorSource;

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
            Shorts = 0x200,
            InterruptEnableSet = 0x304,
            InterruptEnableClear = 0x308,
            ErrorSource = 0x4C4,
            Enable = 0x500,
            RxdPtr = 0x534,
            RxdMaxCnt = 0x538,
            RxdAmount = 0x53C,
            TxdPtr = 0x544,
            TxdMaxCnt = 0x548,
            TxdAmount = 0x54C,
            Address = 0x588,
        }
    }
}
