//
// CryptoCell — the nRF52840's CC310, and specifically its public-key accelerator.
//
// Renode's nRF52840 platform maps nothing above 0x50000800, so this block
// answered nothing at all. MeshCore compiles -D USE_CC310_HW_CRYPTO=1 for every
// nRF52 board, and three of the five published images configure it and then
// poll PKA_DONE until the run ends - upwards of 120 million reads from a
// program counter that never moves again.
//
// The block identified itself by its own arithmetic. The firmware writes 32
// consecutive registers with values exactly 18 apart, then writes 576 into
// seven length registers, and 576 bits is 18 words: a PKA laying out 32 virtual
// registers across its SRAM at the size it is about to work in. The value it
// writes to 0x084 settles it - 0xFF820 unpacks as N=0, Np=1, T0=30, T1=31,
// which is CryptoCell's own default register assignment and nothing else's.
//
// What it computes is Ed25519 signature verification: Mesh.cpp calls
// Identity::verify on the advert receive path, which is CRYS_ECEDW_Verify.
// MeshCore's own comment says why it prefers the hardware there - the software
// Ed25519 wants about 3 KB of stack and can overflow the Adafruit core's 4 KB
// loop task from that exact path.
//
// So this model does the arithmetic rather than claiming to have done it. An
// accelerator that returns a plausible wrong answer is worse than one that
// hangs: the hang is loud, and a signature that verifies when it should not is
// silent. Every operation here is exact integer arithmetic on BigInteger, and
// the opcode set and field positions follow the CC312 runtime, which Arm
// publishes and whose PKA is the same machine grown up.
//
// What is not modelled: timing. Every operation completes before the firmware
// can look, so PKA_DONE is always set. A study of how long crypto takes on this
// part would be measuring this file rather than the hardware.
//
using System.Collections.Generic;
using System.Numerics;
using Antmicro.Renode.Core;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals.Bus;

namespace Antmicro.Renode.Peripherals.Miscellaneous
{
    public class NRF52840_CryptoCell : IDoubleWordPeripheral, IKnownSize
    {
        public NRF52840_CryptoCell(IMachine machine)
        {
            this.machine = machine;
            Reset();
        }

        // 0x5002B000, one page. The CRYPTOCELL wrapper's ENABLE sits a page
        // below at 0x5002A500 and Renode already absorbs it from the SVD, so
        // this block is exactly the part nothing answers today.
        public long Size => 0x1000;

        public uint ReadDoubleWord(long offset)
        {
            switch(offset)
            {
                case PkaDone:
                case PkaPipeRdy:
                    // Nothing here takes time, so these are always ready. If
                    // the firmware reads one of them a hundred thousand times
                    // anyway, it is stuck on something this model got wrong,
                    // and that is worth a line.
                    Watch(offset);
                    return 1;
                case PkaStatus:
                    Watch(offset);
                    return status;
                case PkaSramRdata:
                    return ReadSram();
            }
            if(offset < MemoryMapEnd)
            {
                return map[offset / 4];
            }
            if(offset >= PkaL0 && offset < PkaL0 + 8 * 4)
            {
                return bits[(offset - PkaL0) / 4];
            }
            uint value;
            other.TryGetValue(offset, out value);
            Watch(offset);
            Note("read ", offset, value);
            return value;
        }

        public void WriteDoubleWord(long offset, uint value)
        {
            // Any write ends a poll: a loop that reads and writes is making
            // progress, however slowly, and is not what a stall looks like.
            polling = -1;
            polls = 0;

            if(offset < MemoryMapEnd)
            {
                map[offset / 4] = value;
                return;
            }
            if(offset >= PkaL0 && offset < PkaL0 + 8 * 4)
            {
                bits[(offset - PkaL0) / 4] = value;
                return;
            }
            switch(offset)
            {
                case PkaOpcode:
                    Execute(value);
                    return;
                case PkaNNpT0T1:
                    slots = value;
                    return;
                case PkaSramWaddr:
                    writeAt = value;
                    return;
                case PkaSramRaddr:
                    readAt = value;
                    return;
                case PkaSramWdata:
                    WriteSram(value);
                    return;
                case PkaSwReset:
                    Reset();
                    return;
            }
            other[offset] = value;
            Note("write", offset, value);
        }

        public void Reset()
        {
            for(var i = 0; i < sram.Length; i++)
            {
                sram[i] = 0;
            }
            for(var i = 0; i < map.Length; i++)
            {
                map[i] = 0;
            }
            for(var i = 0; i < bits.Length; i++)
            {
                bits[i] = 0;
            }
            other.Clear();
            seen.Clear();
            said.Clear();
            ops.Clear();
            writeAt = 0;
            readAt = 0;
            status = 0;
            slots = 0;
            polling = -1;
            polls = 0;
        }

        // --- the arithmetic ------------------------------------------------

        private void Execute(uint word)
        {
            var op = (word >> 27) & 0x1F;
            var lenId = (int)((word >> 24) & 0x7);
            var aImmediate = ((word >> 23) & 1) != 0;
            var a = (int)((word >> 18) & 0x1F);
            var bImmediate = ((word >> 17) & 1) != 0;
            var b = (int)((word >> 12) & 0x1F);
            var discard = ((word >> 11) & 1) != 0;
            var r = (int)((word >> 6) & 0x1F);
            // The multiply-accumulate opcodes need a third operand, and the
            // encoding has no room left for one: Arm puts it in the tag field,
            // which every other opcode ignores.
            var c = (int)(word & 0x1F);

            Count(op);
            if(ops[op] == 1)
            {
                this.Log(LogLevel.Warning, "cc310 pka first {0} (0x{1:X2})", Name(op), op);
            }

            if(op == OpTerminate)
            {
                return;
            }

            var wordsInRegister = Words(lenId);
            var A = aImmediate ? new BigInteger(a) : Load(a, wordsInRegister);
            var B = bImmediate ? new BigInteger(b) : Load(b, wordsInRegister);
            var N = Load((int)(slots & 0x1F), wordsInRegister);
            // The shift opcodes carry their count in the operand-2 field and
            // shift by one more than it says, which is how a zero-length shift
            // is kept off the encoding.
            var shift = b + 1;

            BigInteger result;
            var carry = false;
            switch(op)
            {
                case OpAdd: result = A + B; break;
                case OpSub: result = A - B; break;
                case OpModAdd: result = Reduce(A + B, N); break;
                case OpModSub: result = Reduce(A - B, N); break;
                case OpAnd: result = A & B; break;
                case OpOr: result = A | B; break;
                case OpXor: result = A ^ B; break;
                case OpShr0: result = A >> shift; break;
                case OpShr1: result = (A >> shift) | (Ones(shift) << (wordsInRegister * 32 - shift)); break;
                case OpShl0: result = A << shift; break;
                case OpShl1: result = (A << shift) | Ones(shift); break;
                case OpMulLow: result = A * B; break;
                // Not the high half: the high half *plus the top word of the
                // low half*, which is why Arm's comment says the result is one
                // word wider than the operation. Masking it to the operation
                // width throws away the word the caller came for.
                case OpMulHigh: result = (A * B) >> ((wordsInRegister - 1) * 32); break;
                case OpModMul:
                case OpModMulN: result = Reduce(A * B, N); break;
                case OpModExp: result = N.IsZero ? BigInteger.Zero : BigInteger.ModPow(A, B, N); break;
                // Res = OpA * OpB + OpC mod N. The no-reduction variant is
                // allowed to leave a few extra bits on rather than doing the
                // last conditional subtraction; a fully reduced answer is
                // inside that allowance, so both reduce.
                case OpModMlac:
                case OpModMlacNr: result = Reduce(A * B + Load(c, wordsInRegister), N); break;
                case OpModInv: result = Invert(B, N); break;
                // Division leaves the remainder behind in its own first
                // operand - Res = OpA / OpB, OpA = OpA mod OpB. A caller that
                // wanted the remainder gets it from where it passed the
                // dividend, and a model that skips this quietly hands back the
                // dividend unchanged.
                case OpDivision:
                    if(B.IsZero)
                    {
                        status = 1u << StatusDivByZero;
                        return;
                    }
                    Store(a, wordsInRegister, A % B);
                    result = A / B;
                    break;
                case OpReduction: result = Reduce(A, N); break;
                default:
                    Unhandled(op, word);
                    return;
            }

            // A result wider than the register is how this machine reports a
            // carry; the register keeps the low words, as the hardware does.
            // MULHIGH is the exception the documentation calls out: its result
            // is one word wider than the operation by design, and the register
            // slots are mapped with the headroom to hold it.
            var outWords = op == OpMulHigh ? wordsInRegister + 1 : wordsInRegister;
            var span = BigInteger.One << (outWords * 32);
            if(result.Sign < 0)
            {
                result += span;
                carry = true;
            }
            else if(result >= span)
            {
                result %= span;
                carry = true;
            }

            status = 0;
            if(result.IsZero)
            {
                status |= 1u << StatusAluOutZero;
            }
            if(carry)
            {
                status |= 1u << StatusAluCarry;
            }
            if(!discard)
            {
                Store(r, outWords, result);
            }
        }

        // Reduce is not BigInteger's % : that follows the sign of the dividend,
        // and a modular subtraction that went negative must come back positive
        // or every later operation is wrong by exactly N.
        private static BigInteger Reduce(BigInteger v, BigInteger n)
        {
            if(n.IsZero)
            {
                return v;
            }
            var m = v % n;
            return m.Sign < 0 ? m + n : m;
        }

        private static BigInteger Ones(int count)
        {
            return (BigInteger.One << count) - 1;
        }

        // Extended Euclid, because BigInteger has no modular inverse and
        // ModPow(v, n-2, n) is only the inverse when n is prime - which the
        // caller has not promised.
        private static BigInteger Invert(BigInteger v, BigInteger n)
        {
            if(n.IsZero || n.IsOne)
            {
                return BigInteger.Zero;
            }
            BigInteger t = 0, newT = 1, r = n, newR = Reduce(v, n);
            while(!newR.IsZero)
            {
                var q = r / newR;
                var tmp = t - q * newT;
                t = newT;
                newT = tmp;
                tmp = r - q * newR;
                r = newR;
                newR = tmp;
            }
            if(r > BigInteger.One)
            {
                return BigInteger.Zero; // not invertible
            }
            return Reduce(t, n);
        }

        private int Words(int lenId)
        {
            var n = (int)((bits[lenId] + 31) / 32);
            return n < 1 ? 1 : n;
        }

        // Operands sit in SRAM least significant word first. The trailing zero
        // byte keeps BigInteger from reading the top bit as a sign.
        private BigInteger Load(int reg, int words)
        {
            var raw = new byte[words * 4 + 1];
            var at = map[reg & 0x1F];
            for(var i = 0; i < words; i++)
            {
                var w = At(at + (uint)i);
                raw[i * 4 + 0] = (byte)w;
                raw[i * 4 + 1] = (byte)(w >> 8);
                raw[i * 4 + 2] = (byte)(w >> 16);
                raw[i * 4 + 3] = (byte)(w >> 24);
            }
            return new BigInteger(raw);
        }

        // A register is as wide as the widest length the firmware configured,
        // and writing a narrow result into it clears the rest - the slots are
        // mapped that far apart for exactly that reason. Leaving the top words
        // alone lets a wide read pick up the tail of whatever was there before,
        // which the firmware has no way to see coming: it mixes len0 (16 words)
        // and len1 (18 words) on the same registers throughout.
        private int SlotWords()
        {
            var w = 1;
            for(var i = 0; i < bits.Length; i++)
            {
                var n = Words(i);
                if(n > w)
                {
                    w = n;
                }
            }
            return w;
        }

        private void Store(int reg, int words, BigInteger value)
        {
            var raw = value.ToByteArray();
            var at = map[reg & 0x1F];
            var slot = SlotWords();
            for(var i = words; i < slot; i++)
            {
                Put(at + (uint)i, 0);
            }
            for(var i = 0; i < words; i++)
            {
                uint w = 0;
                for(var b = 0; b < 4; b++)
                {
                    var k = i * 4 + b;
                    if(k < raw.Length)
                    {
                        w |= (uint)raw[k] << (8 * b);
                    }
                }
                Put(at + (uint)i, w);
            }
        }

        private uint At(uint word)
        {
            return word < sram.Length ? sram[word] : 0;
        }

        private void Put(uint word, uint value)
        {
            if(word < sram.Length)
            {
                sram[word] = value;
            }
        }

        private uint ReadSram()
        {
            var v = At(readAt);
            readAt++;
            return v;
        }

        private void WriteSram(uint value)
        {
            Put(writeAt, value);
            writeAt++;
        }

        // --- saying what happened ------------------------------------------

        // Anything this model does not have a name for is worth a line: an
        // unmodelled block that answers zero is silently wrong, which is the
        // failure this file exists to avoid.
        private void Note(string what, long offset, uint value)
        {
            int n;
            said.TryGetValue(offset, out n);
            said[offset] = n + 1;
            if(n >= NoteBudget)
            {
                return;
            }
            var last = n + 1 == NoteBudget ? "  (quiet from here)" : string.Empty;
            this.Log(LogLevel.Warning, "cc310 unmodelled {0} 0x{1:X3} = 0x{2:X8}{3}", what, offset, value, last);
        }


        // Opcode names, so a log line says what the firmware wanted rather than
        // a number the reader has to go and look up.
        private static string Name(uint op)
        {
            switch(op)
            {
                case OpTerminate: return "TERMINATE";
                case OpAdd: return "ADD";
                case OpSub: return "SUB";
                case OpModAdd: return "MODADD";
                case OpModSub: return "MODSUB";
                case OpAnd: return "AND";
                case OpOr: return "OR";
                case OpXor: return "XOR";
                case OpShr0: return "SHR0";
                case OpShr1: return "SHR1";
                case OpShl0: return "SHL0";
                case OpShl1: return "SHL1";
                case OpMulLow: return "MULLOW";
                case OpModMul: return "MODMUL";
                case OpModMulN: return "MODMULN";
                case OpModExp: return "MODEXP";
                case OpDivision: return "DIVISION";
                case OpModInv: return "MODINV";
                case OpMulHigh: return "MULHIGH";
                case OpModMlac: return "MODMLAC";
                case OpModMlacNr: return "MODMLACNR";
                case OpReduction: return "REDUCTION";
                default: return "unnamed";
            }
        }

        // Each opcode announces itself once. A firmware that suddenly asks for
        // something new is worth one line; asking ten thousand times is not.
        private void Count(uint op)
        {
            int n;
            ops.TryGetValue(op, out n);
            ops[op] = n + 1;
        }

        private void Unhandled(uint op, uint word)
        {
            int n;
            seen.TryGetValue(op, out n);
            seen[op] = n + 1;
            if(n >= UnhandledBudget)
            {
                return;
            }
            this.Log(LogLevel.Error,
                "cc310 pka opcode 0x{0:X2} is not implemented (word 0x{1:X8}) - the answer it wanted is missing, not wrong",
                op, word);
        }

        // Once this block is mapped, a firmware stuck on it stops producing
        // Renode's "non existing peripheral" warning, and the board matrix
        // loses the only signature it had for a stopped CPU. So the model says
        // so itself.
        private void Watch(long offset)
        {
            if(offset != polling)
            {
                polling = offset;
                polls = 0;
                return;
            }
            polls++;
            if(polls == StallAfter)
            {
                this.Log(LogLevel.Warning,
                    "cc310 stalled: {0} reads of 0x{1:X3} with nothing written between", polls, offset);
            }
        }

        // What the firmware actually asked for, said once rather than a line at
        // a time - a verification is thousands of operations and a log line
        // each would bury the run.

        private const long MemoryMapEnd = 0x080;
        private const long PkaOpcode = 0x080;
        private const long PkaNNpT0T1 = 0x084;
        private const long PkaStatus = 0x088;
        private const long PkaSwReset = 0x08C;
        private const long PkaL0 = 0x090;
        private const long PkaPipeRdy = 0x0B0;
        private const long PkaDone = 0x0B4;
        // Read off a running board, not from a datasheet: the firmware sets a
        // write address at 0x0D4 and pushes words at 0x0D8, sets a read address
        // at 0x0E4 and pulls them back from 0x0DC. Both cursors advance a word
        // at a time, and they are independent of each other - which is why
        // there are two and not one.
        private const long PkaSramWaddr = 0x0D4;
        private const long PkaSramWdata = 0x0D8;
        private const long PkaSramRdata = 0x0DC;
        private const long PkaSramRaddr = 0x0E4;

        private const uint OpTerminate = 0x00;
        private const uint OpAdd = 0x04;
        private const uint OpSub = 0x05;
        private const uint OpModAdd = 0x06;
        private const uint OpModSub = 0x07;
        private const uint OpAnd = 0x08;
        private const uint OpOr = 0x09;
        private const uint OpXor = 0x0A;
        private const uint OpShr0 = 0x0C;
        private const uint OpShr1 = 0x0D;
        private const uint OpShl0 = 0x0E;
        private const uint OpShl1 = 0x0F;
        private const uint OpMulLow = 0x10;
        private const uint OpModMul = 0x11;
        private const uint OpModMulN = 0x12;
        private const uint OpModExp = 0x13;
        private const uint OpDivision = 0x14;
        private const uint OpModInv = 0x15;
        private const uint OpMulHigh = 0x17;
        private const uint OpModMlac = 0x18;
        private const uint OpModMlacNr = 0x19;
        private const uint OpReduction = 0x1B;

        private const int StatusAluCarry = 9;
        private const int StatusAluOutZero = 12;
        private const int StatusDivByZero = 14;

        private const int NoteBudget = 10;
        private const int UnhandledBudget = 3;
        private const int StallAfter = 100000;

        // The firmware maps 32 registers of 18 words. Twice that is room for
        // anything it might map instead, and it is four kilobytes.
        private const int SramWords = 1024;

        private readonly uint[] sram = new uint[SramWords];
        private readonly uint[] map = new uint[32];
        private readonly uint[] bits = new uint[8];
        private readonly Dictionary<long, uint> other = new Dictionary<long, uint>();
        private readonly Dictionary<long, int> said = new Dictionary<long, int>();
        private readonly Dictionary<uint, int> ops = new Dictionary<uint, int>();
        private readonly Dictionary<uint, int> seen = new Dictionary<uint, int>();

        private uint writeAt;
        private uint readAt;
        private uint status;
        private uint slots;
        private long polling;
        private int polls;
        private readonly IMachine machine;
    }
}
