//
// Where the nRF52840 puts its USB registers, and the EasyDMA slot an endpoint
// moves bytes through.
//
// Apart from the controller because both halves of it carry a trap that cost a
// day, and neither is about behaviour - it is arithmetic, and arithmetic is
// worth being able to read on its own.
//
// The task block starts one word in. 0x000 is reserved, so TASKS_STARTEPIN[0]
// is at 0x004 and everything after it shifts by a word. A model that takes the
// block as starting at zero puts every endpoint's task on its neighbour's, and
// what that looks like from outside is a device answering a control transfer on
// endpoint 1 - a stack fault, apparently, rather than an address being wrong.
//
// And an endpoint's PTR, MAXCNT and AMOUNT sit in a five-word slot, so the
// stride is 0x14 and not the 0x10 the shape invites.
//
namespace Antmicro.Renode.Peripherals.USB
{
    public static class Usbd
    {
        public const int Endpoints = 8;

        // The event registers run from EVENTS_USBRESET at 0x100, one word each,
        // in the same order as the interrupt-enable bits. The driver walks them
        // by that index, so the two orders have to stay the one order.
        public const int EventCount = 25;
        public const int EvUsbReset = 0;
        public const int EvStarted = 1;
        public const int EvEndEpIn = 2;
        public const int EvEp0DataDone = 10;
        public const int EvEndEpOut = 12;
        public const int EvSof = 21;
        public const int EvEp0Setup = 23;
        public const int EvEpData = 24;

        public const long StartEpIn = 0x004;
        public const long StartEpOut = 0x028;
        public const long Ep0RcvOut = 0x04C;
        public const long Ep0Status = 0x050;
        public const long Ep0Stall = 0x054;
        public const long EventsBase = 0x100;
        public const long InterruptEnable = 0x300;
        public const long InterruptEnableSet = 0x304;
        public const long InterruptEnableClear = 0x308;
        public const long EventCause = 0x400;
        public const long EpDataStatus = 0x46C;
        public const long UsbAddress = 0x470;
        public const long SetupBase = 0x480;
        public const long SizeEpOut = 0x4A0;
        public const long Enable = 0x500;
        public const long PullUp = 0x504;
        public const long EpInEnable = 0x510;
        public const long EpOutEnable = 0x514;
        public const long FrameCounter = 0x520;
        public const long EpInBase = 0x600;
        public const long EpOutBase = 0x700;

        public const uint EventCauseReady = 1u << 11;
    }

    // One direction's eight EasyDMA slots: where the firmware wants the bytes,
    // how many it will take, and how many it got.
    public class UsbdEndpointSlots
    {
        public UsbdEndpointSlots(long baseOffset)
        {
            this.baseOffset = baseOffset;
        }

        public ulong Pointer(int endpoint) => pointer[endpoint];
        public int Count(int endpoint) => (int)count[endpoint];
        public void Took(int endpoint, int bytes) => amount[endpoint] = (uint)bytes;

        public void Clear()
        {
            for(var i = 0; i < Usbd.Endpoints; i++)
            {
                pointer[i] = 0;
                count[i] = amount[i] = 0;
            }
        }

        public bool TryRead(long offset, out uint value)
        {
            value = 0;
            int endpoint, field;
            if(!Locate(offset, out endpoint, out field))
            {
                return false;
            }
            value = field == 0 ? (uint)pointer[endpoint]
                  : field == 1 ? count[endpoint]
                  : amount[endpoint];
            return true;
        }

        public bool TryWrite(long offset, uint value)
        {
            int endpoint, field;
            if(!Locate(offset, out endpoint, out field))
            {
                return false;
            }
            if(field == 0)
            {
                pointer[endpoint] = value;
            }
            else if(field == 1)
            {
                count[endpoint] = value;
            }
            return true;
        }

        private bool Locate(long offset, out int endpoint, out int field)
        {
            endpoint = field = 0;
            if(offset < baseOffset || offset >= baseOffset + Stride * Usbd.Endpoints)
            {
                return false;
            }
            var delta = offset - baseOffset;
            endpoint = (int)(delta / Stride);
            field = (int)((delta % Stride) / 4);
            return field < 3;
        }

        private const long Stride = 0x14;

        private readonly long baseOffset;
        private readonly ulong[] pointer = new ulong[Usbd.Endpoints];
        private readonly uint[] count = new uint[Usbd.Endpoints];
        private readonly uint[] amount = new uint[Usbd.Endpoints];
    }
}
