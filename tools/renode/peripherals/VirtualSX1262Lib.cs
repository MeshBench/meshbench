//
// Loading virtual-sx1262, and the ABI it exposes.
//
// The chip is a C library, and this is the declaration of it. Two things are
// done the long way round here on purpose.
//
// The library is opened by path rather than by a DllImport name, because the
// path is decided by whoever started this emulator and named in
// MESHBENCH_RADIO_LIB. A DllImport name would need the file on the loader's
// search path, which means an environment variable set around Renode on Linux,
// a different one on macOS and a directory on Windows - three ways to get the
// same thing subtly wrong, for a file whose location we already know.
//
// And the entry points are delegates over dlsym rather than DllImport
// declarations, because Renode compiles this file at load time and may be
// running on Mono or on .NET. NativeLibrary and its resolver are not on both.
// dlopen and LoadLibrary are.
//
// The ABI is a contract, and the other side of it is
// MeshBench/virtual-sx1262's include/virtual_sx1262.h. It is append-only:
// never reorder a struct, never change what an entry point means. This
// declares ABI 1.3, the version that added the byte-at-a-time SPI path, which
// is the only one an emulator can use - Renode's ISPIPeripheral.Transmit is
// called once per clocked byte and must answer that byte before the next.
//
using System;
using System.Runtime.InteropServices;

namespace Antmicro.Renode.Peripherals.Radio
{
    // Laid out to match vsx_state exactly. Sequential, and no field may be
    // reordered or resized: a mismatch here does not fail, it reads one
    // setting as another.
    [StructLayout(LayoutKind.Sequential)]
    public struct VsxState
    {
        public uint FreqHz;
        public uint BandwidthHz;
        public ushort PreambleSyms;
        public ushort IrqMask;
        public ushort IrqFlags;
        public byte SpreadingFactor;
        public byte CodingRate;
        public byte Mode;          // 0 standby, 1 rx, 2 tx, 3 cad
        public sbyte TxPowerDbm;
        public byte RxGainReg;
        public byte FemAtTx;       // 0 never transmitted, 1 module out, 2 module in
        public ushort Dio1Mask;
    }

    [StructLayout(LayoutKind.Sequential)]
    public struct VsxCounters
    {
        public uint IrqReads;
        public uint BusyReads;
        public uint BusyMs;
        public uint SpuriousRaises;
        public uint PreambleRaises;
        public uint FramesDropped;
    }

    [UnmanagedFunctionPointer(CallingConvention.Cdecl)]
    public delegate void VsxDio1Callback(IntPtr user, int asserted);

    public class VirtualSX1262Lib
    {
        public const int AbiMajor = 1;
        public const int AbiMinor = 3;

        public static VirtualSX1262Lib Open(string path)
        {
            var handle = NativeOpen(path);
            if(handle == IntPtr.Zero)
            {
                throw new InvalidOperationException(
                    string.Format("cannot load the chip model at {0}: {1}", path, NativeError()));
            }
            var lib = new VirtualSX1262Lib(handle, path);
            int major, minor;
            lib.AbiVersion(out major, out minor);
            // A major that does not match means every host has to be rebuilt,
            // and the library is saying so; a minor below the floor means an
            // entry point this peripheral calls is not there. Refusing is the
            // point of asking: an unchecked mismatch is a chip that answers
            // plausible nonsense.
            if(major != AbiMajor || minor < AbiMinor)
            {
                throw new InvalidOperationException(string.Format(
                    "{0} is ABI {1}.{2}, and this peripheral needs {3}.{4} or a later minor",
                    path, major, minor, AbiMajor, AbiMinor));
            }
            return lib;
        }

        public string Path { get; private set; }

        public AbiVersionFn AbiVersion;
        public CreateFn Create;
        public DestroyFn Destroy;
        public SetDio1CallbackFn SetDio1Callback;
        public SpiBeginFn SpiBegin;
        public SpiByteFn SpiByte;
        public SpiEndFn SpiEnd;
        public BusyFn Busy;
        public TickFn Tick;
        public SetChannelBusyFn SetChannelBusy;
        public DeliverFrameFn DeliverFrame;
        public TransmitFinishedFn TransmitFinished;
        public TakeTxFn TakeTx;
        public SetFemEnabledFn SetFemEnabled;
        public GetStateFn GetState;
        public GetCountersFn GetCounters;
        public SetNoiseSeedFn SetNoiseSeed;

        public delegate void AbiVersionFn(out int major, out int minor);
        public delegate IntPtr CreateFn();
        public delegate void DestroyFn(IntPtr chip);
        public delegate void SetDio1CallbackFn(IntPtr chip, VsxDio1Callback fn, IntPtr user);
        public delegate void SpiBeginFn(IntPtr chip);
        public delegate byte SpiByteFn(IntPtr chip, byte outByte);
        public delegate void SpiEndFn(IntPtr chip);
        public delegate int BusyFn(IntPtr chip);
        public delegate void TickFn(IntPtr chip, ulong nowMs);
        public delegate void SetChannelBusyFn(IntPtr chip, int busy);
        public delegate void DeliverFrameFn(IntPtr chip, byte[] frame, UIntPtr len);
        public delegate void TransmitFinishedFn(IntPtr chip);
        public delegate UIntPtr TakeTxFn(IntPtr chip, byte[] dst, UIntPtr cap);
        public delegate void SetFemEnabledFn(IntPtr chip, int enabled);
        public delegate void GetStateFn(IntPtr chip, out VsxState state);
        public delegate void GetCountersFn(IntPtr chip, out VsxCounters counters);
        public delegate void SetNoiseSeedFn(IntPtr chip, ulong seed);

        private VirtualSX1262Lib(IntPtr handle, string path)
        {
            this.handle = handle;
            Path = path;

            AbiVersion = (AbiVersionFn)Bind("vsx_abi_version", typeof(AbiVersionFn));
            Create = (CreateFn)Bind("vsx_create", typeof(CreateFn));
            Destroy = (DestroyFn)Bind("vsx_destroy", typeof(DestroyFn));
            SetDio1Callback = (SetDio1CallbackFn)Bind("vsx_set_dio1_callback", typeof(SetDio1CallbackFn));
            SpiBegin = (SpiBeginFn)Bind("vsx_spi_begin", typeof(SpiBeginFn));
            SpiByte = (SpiByteFn)Bind("vsx_spi_byte", typeof(SpiByteFn));
            SpiEnd = (SpiEndFn)Bind("vsx_spi_end", typeof(SpiEndFn));
            Busy = (BusyFn)Bind("vsx_busy", typeof(BusyFn));
            Tick = (TickFn)Bind("vsx_tick", typeof(TickFn));
            SetChannelBusy = (SetChannelBusyFn)Bind("vsx_set_channel_busy", typeof(SetChannelBusyFn));
            DeliverFrame = (DeliverFrameFn)Bind("vsx_deliver_frame", typeof(DeliverFrameFn));
            TransmitFinished = (TransmitFinishedFn)Bind("vsx_transmit_finished", typeof(TransmitFinishedFn));
            TakeTx = (TakeTxFn)Bind("vsx_take_tx", typeof(TakeTxFn));
            SetFemEnabled = (SetFemEnabledFn)Bind("vsx_set_fem_enabled", typeof(SetFemEnabledFn));
            GetState = (GetStateFn)Bind("vsx_get_state", typeof(GetStateFn));
            GetCounters = (GetCountersFn)Bind("vsx_get_counters", typeof(GetCountersFn));
            SetNoiseSeed = (SetNoiseSeedFn)Bind("vsx_set_noise_seed", typeof(SetNoiseSeedFn));
        }

        private Delegate Bind(string name, Type type)
        {
            var symbol = NativeSymbol(handle, name);
            if(symbol == IntPtr.Zero)
            {
                throw new InvalidOperationException(
                    string.Format("{0} has no {1}: is it virtual-sx1262?", Path, name));
            }
            return Marshal.GetDelegateForFunctionPointer(symbol, type);
        }

        // The platform's loader, by hand. Windows is the odd one; everywhere
        // else is dlopen, and macOS resolves it in libSystem.
        private static bool OnWindows
        {
            get
            {
                var p = (int)Environment.OSVersion.Platform;
                return p != 4 && p != 6 && p != 128;
            }
        }

        private static IntPtr NativeOpen(string path)
        {
            // RTLD_NOW | RTLD_LOCAL: every symbol is resolved below anyway, and
            // a missing one should be named here rather than crash the machine
            // at the first SPI byte.
            return OnWindows ? LoadLibraryW(path) : dlopen(path, 2);
        }

        private static IntPtr NativeSymbol(IntPtr handle, string name)
        {
            return OnWindows ? GetProcAddress(handle, name) : dlsym(handle, name);
        }

        private static string NativeError()
        {
            if(OnWindows)
            {
                return string.Format("error {0}", Marshal.GetLastWin32Error());
            }
            var err = dlerror();
            return err == IntPtr.Zero ? "no reason given" : Marshal.PtrToStringAnsi(err);
        }

        private readonly IntPtr handle;

        [DllImport("libdl.so.2", EntryPoint = "dlopen")]
        private static extern IntPtr dlopen(string path, int flags);
        [DllImport("libdl.so.2", EntryPoint = "dlsym")]
        private static extern IntPtr dlsym(IntPtr handle, string name);
        [DllImport("libdl.so.2", EntryPoint = "dlerror")]
        private static extern IntPtr dlerror();

        [DllImport("kernel32", SetLastError = true, CharSet = CharSet.Unicode)]
        private static extern IntPtr LoadLibraryW(string path);
        [DllImport("kernel32", SetLastError = true)]
        private static extern IntPtr GetProcAddress(IntPtr handle, string name);
    }
}
