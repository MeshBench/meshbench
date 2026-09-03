//
// NVMC - the nRF52840's non-volatile memory controller, which Renode has not
// got at all.
//
// Renode's nrf52840 platform maps flash as plain writable memory and stops
// there, so the controller that erases and programs it answers only from the
// SVD: every register reads back a default and every task goes nowhere. The
// firmware notices. Adafruit's InternalFS is LittleFS on internal flash, and
// LittleFS cannot format a volume it cannot erase, so MeshCore comes up saying
//
//     DEBUG: Generating new keypair
//     DEBUG: IdentityStore::save() failed
//
// on every boot. A repeater with no filesystem has no preferences and no stored
// identity, which is not a small thing: simple_repeater's allowPacketForward
// reads its region transport codes, loop-detect table and hop caps out of them,
// and a board that hears every advert and forwards none looks for all the world
// like a broken radio. It is a missing peripheral three layers down.
//
// It also explains why the boards regenerate the same identity: with nothing
// saved they make a fresh keypair each boot, and the Arduino core seeds its PRNG
// from the radio, which answers the same number to every node.
//
// What is modelled: READY, the erase tasks, and CONFIG. What is not: timing.
// A real page erase takes about 85 ms and a word write about 41 us, and the
// firmware polls READY rather than waiting on an interrupt, so completing at
// once is early rather than wrong - the same choice NRF52840_Clock makes, and
// for the same reason. Nothing a result depends on is timed here; the RF engine
// owns that.
//
// Write protection is deliberately not enforced. On the part, programming needs
// CONFIG.WEN and a write without it is dropped. Here the bus writes straight
// into mapped memory whatever CONFIG says, because Renode gives the peripheral
// no way to intercept them - so a firmware bug that programs without enabling
// writes would pass here and fail on copper. That is a gap worth knowing about
// rather than one worth pretending away.
//
using System;
using Antmicro.Renode.Core;
using Antmicro.Renode.Core.Structure.Registers;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals.Bus;

namespace Antmicro.Renode.Peripherals.MTD
{
    public class NRF52840_NVMC : BasicDoubleWordPeripheral, IKnownSize
    {
        public NRF52840_NVMC(IMachine machine) : base(machine)
        {
            DefineRegisters();
        }

        public long Size => 0x1000;

        private void DefineRegisters()
        {
            // Ready, always. The erase below happens inside the write that asks
            // for it, so there is never a moment when it is not.
            Registers.Ready.Define(this)
                .WithFlag(0, FieldMode.Read, valueProviderCallback: _ => true, name: "READY")
                .WithReservedBits(1, 31)
            ;
            Registers.ReadyNext.Define(this)
                .WithFlag(0, FieldMode.Read, valueProviderCallback: _ => true, name: "READYNEXT")
                .WithReservedBits(1, 31)
            ;

            Registers.Config.Define(this)
                .WithEnumField<DoubleWordRegister, ConfigMode>(0, 2, out config, name: "WEN")
                .WithReservedBits(2, 30)
            ;

            // The address of any word in the page decides the page: the part
            // ignores the low bits, and firmware relies on that rather than
            // aligning first.
            Registers.ErasePage.Define(this)
                .WithValueField(0, 32, FieldMode.Write, name: "ERASEPAGE",
                    writeCallback: (_, val) => ErasePage((ulong)val))
            ;
            Registers.ErasePcr0.Define(this)
                .WithValueField(0, 32, FieldMode.Write, name: "ERASEPCR0",
                    writeCallback: (_, val) => ErasePage((ulong)val))
            ;

            // A partial erase is a whole erase here. It exists so a long erase
            // can be broken up without starving an interrupt, which is a timing
            // concern and this model has no timing.
            Registers.ErasePagePartial.Define(this)
                .WithValueField(0, 32, FieldMode.Write, name: "ERASEPAGEPARTIAL",
                    writeCallback: (_, val) => ErasePage((ulong)val))
            ;
            Registers.ErasePagePartialCfg.Define(this)
                .WithValueField(0, 7, name: "DURATION")
                .WithReservedBits(7, 25)
            ;

            Registers.EraseAll.Define(this)
                .WithFlag(0, FieldMode.Write, name: "ERASEALL", writeCallback: (_, val) =>
                {
                    if(val)
                    {
                        Erase(FlashStart, FlashSize);
                        this.Log(LogLevel.Info, "ERASEALL: the whole of flash is now blank");
                    }
                })
                .WithReservedBits(1, 31)
            ;

            Registers.EraseUicr.Define(this)
                .WithFlag(0, FieldMode.Write, name: "ERASEUICR", writeCallback: (_, val) =>
                {
                    if(val)
                    {
                        Erase(UicrStart, UicrSize);
                    }
                })
                .WithReservedBits(1, 31)
            ;

            // The instruction cache. Nothing here caches anything, so these are
            // accepted and counted at zero rather than left to the SVD, which
            // logs a warning on every access and buries the interesting ones.
            Registers.ICacheConfig.Define(this)
                .WithFlag(0, name: "CACHEEN")
                .WithReservedBits(1, 7)
                .WithFlag(8, name: "CACHEPROFEN")
                .WithReservedBits(9, 23)
            ;
            Registers.ICacheHit.Define(this)
                .WithValueField(0, 32, FieldMode.Read, valueProviderCallback: _ => 0, name: "HITS")
            ;
            Registers.ICacheMiss.Define(this)
                .WithValueField(0, 32, FieldMode.Read, valueProviderCallback: _ => 0, name: "MISSES")
            ;
        }

        private void ErasePage(ulong address)
        {
            var page = address & ~(PageSize - 1);
            if(page >= FlashStart + FlashSize)
            {
                // Out of range rather than fatal: the UICR has its own task, and
                // a stray address is the firmware's mistake to see, not a reason
                // to take the machine down.
                this.Log(LogLevel.Warning, "ERASEPAGE for 0x{0:X}, which is not in flash", address);
                return;
            }
            Erase(page, PageSize);
        }

        private void Erase(ulong start, ulong size)
        {
            // Blank is 0xFF: the part erases to ones and programming only clears
            // bits. LittleFS checks for that pattern to decide a block is free,
            // so zeroing here would leave it reading a volume full of data it
            // could not parse.
            var blank = new byte[PageSize];
            for(var i = 0; i < blank.Length; i++)
            {
                blank[i] = 0xFF;
            }
            for(ulong at = start; at < start + size; at += PageSize)
            {
                var run = Math.Min(PageSize, start + size - at);
                sysbus.WriteBytes(blank, at, (int)run);
            }
        }

        private IEnumRegisterField<ConfigMode> config;

        private const ulong PageSize = 0x1000;
        private const ulong FlashStart = 0x0;
        private const ulong FlashSize = 0x100000;
        private const ulong UicrStart = 0x10001000;
        private const ulong UicrSize = 0x1000;

        private enum ConfigMode
        {
            ReadOnly = 0,
            WriteEnable = 1,
            EraseEnable = 2,
        }

        private enum Registers : long
        {
            Ready = 0x400,
            ReadyNext = 0x408,
            Config = 0x504,
            ErasePage = 0x508,
            EraseAll = 0x50C,
            ErasePcr0 = 0x510,
            EraseUicr = 0x514,
            ErasePagePartial = 0x518,
            ErasePagePartialCfg = 0x51C,
            ICacheConfig = 0x540,
            ICacheHit = 0x548,
            ICacheMiss = 0x54C,
        }
    }
}
