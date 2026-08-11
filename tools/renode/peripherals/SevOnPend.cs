//
// SEVONPEND, which Renode's CPU does not implement.
//
// ARM DDI0403E B1.5.17: when SCR.SEVONPEND is set, an interrupt changing to
// pending is an event, and an event wakes WFE — *whether or not that interrupt
// is enabled*. Firmware uses this to sleep until anything at all becomes
// pending, then read ISPR and deal with it in thread mode, without ever taking
// the interrupt. That is why the interrupt is deliberately left disabled.
//
// MeshCore's published nRF52 build does exactly this, and Renode sleeps for
// ever with the wake condition already true:
//
//     SCR    0x00000010   SEVONPEND set
//     ISPR0  0x00020000   RTC1 pending
//     ISER0  0x00000001   RTC1 not enabled
//
// The right fix is in tlib, which owns the event register — there is an open
// upstream issue about a neighbouring case (renode#892, WFE not waking on
// exception return). This is a shim, not that fix: it watches for the exact
// architectural condition and releases the CPU, so the firmware sees the
// behaviour the part would give it.
//
// It is deliberately narrow. It does nothing unless SEVONPEND is set and
// something is actually pending, so a machine that does not use the idiom is
// untouched, and it fabricates no interrupt: the CPU wakes and reads ISPR
// itself, exactly as it would on hardware.
//
using System.Linq;
using Antmicro.Renode.Core;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals.CPU;
using Antmicro.Renode.Peripherals.Timers;
using Antmicro.Renode.Time;

namespace Antmicro.Renode.Peripherals.Miscellaneous
{
    public class SevOnPend : IDoubleWordPeripheral, IKnownSize
    {
        public SevOnPend(IMachine machine)
        {
            this.machine = machine;

            // Polled rather than driven by the NVIC, because a plugin cannot
            // reach inside Renode's interrupt controller. The rate only has to
            // be short against the intervals the firmware is waiting on, which
            // are milliseconds at the fastest.
            timer = new LimitTimer(machine.ClockSource, 1000, this, "sevonpend",
                                   limit: 1, eventEnabled: true,
                                   direction: Direction.Ascending,
                                   enabled: true, workMode: WorkMode.Periodic);
            timer.LimitReached += Check;
        }

        public long Size => 0x4;

        public uint ReadDoubleWord(long offset) => releases;
        public void WriteDoubleWord(long offset, uint value) { }
        public void Reset() => releases = 0;

        private void Check()
        {
            var cpu = machine.SystemBus.GetCPUs().OfType<ICPUWithHooks>().FirstOrDefault();
            if(cpu == null || !cpu.IsHalted)
            {
                return;
            }

            // SCR.SEVONPEND, bit 2 of the System Control Register.
            var scr = machine.SystemBus.ReadDoubleWord(ScrAddress);
            if((scr & SevOnPendBit) == 0)
            {
                return;
            }

            var pending = machine.SystemBus.ReadDoubleWord(Ispr0)
                        | machine.SystemBus.ReadDoubleWord(Ispr1);
            if(pending == 0)
            {
                return;
            }

            // The event, delivered the only way a plugin can: let the CPU run.
            // It will execute the instruction after its WFE, read ISPR, and
            // find what it was waiting for.
            cpu.IsHalted = false;
            releases++;
            if(releases == 1)
            {
                this.Log(LogLevel.Info,
                    "released the CPU from WFE: SEVONPEND set and 0x{0:X8} pending. " +
                    "Renode does not implement SEVONPEND; this stands in for it.",
                    pending);
            }
        }

        private const long ScrAddress = 0xE000ED10;
        private const long Ispr0 = 0xE000E200;
        private const long Ispr1 = 0xE000E204;
        private const uint SevOnPendBit = 1u << 4;

        private readonly IMachine machine;
        private readonly LimitTimer timer;
        private uint releases;
    }
}
