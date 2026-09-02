//
// The other end of the USB cable, which an emulator does not otherwise have.
//
// A device controller is only half a link: nothing in TinyUSB happens until a
// host resets the bus, reads the descriptors, assigns an address and selects a
// configuration. Renode has no USB host, and modelling a general one would be a
// far larger thing than the console this exists for.
//
// So this is a scripted host and not a stack: it performs the enumeration a
// real one performs, in the order a real one performs it, and then it does the
// only thing a console needs - move bytes on the CDC bulk endpoints. It reads
// the configuration descriptor rather than assuming the endpoint numbers,
// because MeshCore's images are built per board and the interface layout is not
// ours to predict.
//
// Anything a real host would also do - string descriptors, status queries,
// suspend, error recovery beyond a restart - is absent. A device that stalls or
// stops answering is taken back to the start rather than nursed, because the
// question this has to answer is "did the board come up", and a half-enumerated
// device that limps is the answer that wastes the most time.
//
// The interface is here rather than beside the controller because Renode
// compiles each included file on its own: the two can depend in one direction
// and not both, and it is the host that has to be loadable first.
//
using System;
using System.Collections.Generic;
using Antmicro.Renode.Core;
using Antmicro.Renode.Logging;

namespace Antmicro.Renode.Peripherals.USB
{
    public interface IUsbCdcDevice : IEmulationElement
    {
        bool Attached { get; }
        void HostReset();
        void HostSetup(byte[] packet);
        void HostControlOut(byte[] data);
        void HostOut(int endpoint, byte[] data);
        bool OutBusy(int endpoint);
        void ToTerminal(byte[] data);
    }

    public class UsbCdcHost
    {
        public UsbCdcHost(IUsbCdcDevice device)
        {
            this.device = device;
            Reset();
        }

        public void Reset()
        {
            step = Step.Detached;
            waited = 0;
            active = false;
            received.Clear();
            commInterface = 0;
            bulkIn = bulkOut = 0;
        }

        // One bus frame. Everything this host initiates happens here, so the
        // whole of it runs on the emulation thread and in the board's own time.
        public void Frame(Queue<byte> typed)
        {
            if(!device.Attached)
            {
                if(step != Step.Detached)
                {
                    Reset();
                }
                return;
            }
            if(step == Step.Detached)
            {
                device.HostReset();
                step = Step.Resetting;
                waited = 0;
                return;
            }
            if(active)
            {
                Stall();
                return;
            }
            switch(step)
            {
                case Step.Resetting:
                    // A real host holds reset and then waits before addressing
                    // the device; the driver needs those frames to run its own
                    // bus_reset before a SETUP lands on it.
                    if(++waited > ResetFrames)
                    {
                        Begin(Step.DeviceDescriptor);
                    }
                    return;
                case Step.Running:
                    Pump(typed);
                    return;
                default:
                    return;
            }
        }

        public void ControlIn(byte[] data)
        {
            foreach(var b in data)
            {
                received.Add(b);
            }
        }

        public void ControlDone()
        {
            if(!active)
            {
                return;
            }
            active = false;
            waited = 0;
            Advance(received.ToArray());
        }

        public void ControlStalled()
        {
            device.Log(LogLevel.Warning, "the device stalled control step {0}; starting again", step);
            active = false;
            step = Step.Detached;
        }

        public void BulkIn(int endpoint, byte[] data)
        {
            if(endpoint == bulkIn && data.Length > 0)
            {
                device.ToTerminal(data);
            }
        }

        // What the enumeration learned decides what is asked next, so each step
        // is answered where its answer arrives rather than in a table.
        private void Advance(byte[] answer)
        {
            switch(step)
            {
                case Step.DeviceDescriptor:
                    Begin(Step.SetAddress);
                    return;
                case Step.SetAddress:
                    Begin(Step.ConfigurationHeader);
                    return;
                case Step.ConfigurationHeader:
                    total = answer.Length >= 4 ? (answer[2] | (answer[3] << 8)) : 0;
                    Begin(total > 0 ? Step.Configuration : Step.Detached);
                    return;
                case Step.Configuration:
                    if(!ReadEndpoints(answer))
                    {
                        device.Log(LogLevel.Error,
                                   "the device's configuration descriptor carries no CDC data " +
                                   "interface, so this board's console is not a serial one");
                        step = Step.Idle;
                        return;
                    }
                    Begin(Step.SetConfiguration);
                    return;
                case Step.SetConfiguration:
                    Begin(Step.LineCoding);
                    return;
                case Step.LineCoding:
                    Begin(Step.LineState);
                    return;
                case Step.LineState:
                    device.Log(LogLevel.Info,
                               "CDC console up: data in on endpoint {0}, out on endpoint {1}",
                               bulkIn, bulkOut);
                    step = Step.Running;
                    return;
                default:
                    step = Step.Idle;
                    return;
            }
        }

        private void Begin(Step next)
        {
            step = next;
            received.Clear();
            switch(next)
            {
                case Step.DeviceDescriptor:
                    Issue(0x80, GetDescriptor, DeviceDescriptorType << 8, 0, 18);
                    return;
                case Step.SetAddress:
                    Issue(0x00, SetAddress, Address, 0, 0);
                    return;
                case Step.ConfigurationHeader:
                    Issue(0x80, GetDescriptor, ConfigDescriptorType << 8, 0, 9);
                    return;
                case Step.Configuration:
                    Issue(0x80, GetDescriptor, ConfigDescriptorType << 8, 0, total);
                    return;
                case Step.SetConfiguration:
                    Issue(0x00, SetConfiguration, 1, 0, 0);
                    return;
                case Step.LineCoding:
                    // 115200 8N1, little endian, then no parity and eight data
                    // bits. The firmware never sees these as timing - it is USB
                    // underneath - but it does see the request, and TinyUSB
                    // treats a CDC that was never configured as one nobody has
                    // opened.
                    Issue(0x21, SetLineCoding, 0, commInterface, 7);
                    device.HostControlOut(new byte[] { 0x00, 0xC2, 0x01, 0x00, 0x00, 0x00, 0x08 });
                    return;
                case Step.LineState:
                    // DTR and RTS. DTR is the one that matters: tud_cdc_connected
                    // is false without it, so a sketch that waits on Serial
                    // waits for ever and a board that prints looks mute.
                    Issue(0x21, SetControlLineState, 0x0003, commInterface, 0);
                    return;
                default:
                    step = Step.Idle;
                    return;
            }
        }

        private void Issue(byte type, byte request, int value, int index, int length)
        {
            var packet = new byte[8];
            packet[0] = type;
            packet[1] = request;
            packet[2] = (byte)value;
            packet[3] = (byte)(value >> 8);
            packet[4] = (byte)index;
            packet[5] = (byte)(index >> 8);
            packet[6] = (byte)length;
            packet[7] = (byte)(length >> 8);
            active = true;
            waited = 0;
            device.HostSetup(packet);
        }

        // A control transfer that never finishes is the failure this model is
        // most likely to produce, and the one hardest to see from outside, so it
        // says which step gave up rather than simply going quiet.
        private void Stall()
        {
            if(++waited <= PatienceFrames)
            {
                return;
            }
            device.Log(LogLevel.Warning,
                       "no answer to USB control step {0} after {1} frames; starting again",
                       step, PatienceFrames);
            active = false;
            step = Step.Detached;
        }

        private void Pump(Queue<byte> typed)
        {
            if(typed.Count == 0 || device.OutBusy(bulkOut))
            {
                return;
            }
            var take = Math.Min(typed.Count, PacketSize);
            var packet = new byte[take];
            for(var i = 0; i < take; i++)
            {
                packet[i] = typed.Dequeue();
            }
            device.HostOut(bulkOut, packet);
        }

        // Walk the configuration descriptor for the CDC data interface's two
        // bulk endpoints and the notification interface the class requests are
        // addressed to. Descriptors are a chain of length-prefixed records, so
        // an unknown one is skipped rather than fatal.
        private bool ReadEndpoints(byte[] desc)
        {
            var inData = false;
            for(var i = 0; i + 1 < desc.Length && desc[i] > 0; i += desc[i])
            {
                if(desc[i + 1] == InterfaceDescriptorType && i + 6 < desc.Length)
                {
                    inData = desc[i + 5] == CdcDataClass;
                    if(desc[i + 5] == CdcCommClass && desc[i + 6] == CdcAbstractControl)
                    {
                        commInterface = desc[i + 2];
                    }
                }
                if(desc[i + 1] != EndpointDescriptorType || !inData || i + 3 >= desc.Length)
                {
                    continue;
                }
                if((desc[i + 3] & 0x03) != BulkTransfer)
                {
                    continue;
                }
                if((desc[i + 2] & 0x80) != 0)
                {
                    bulkIn = desc[i + 2] & 0x0F;
                }
                else
                {
                    bulkOut = desc[i + 2] & 0x0F;
                }
            }
            return bulkIn != 0 && bulkOut != 0;
        }

        private enum Step
        {
            Detached,
            Resetting,
            DeviceDescriptor,
            SetAddress,
            ConfigurationHeader,
            Configuration,
            SetConfiguration,
            LineCoding,
            LineState,
            Running,
            Idle,
        }

        private const int ResetFrames = 10;
        private const int PatienceFrames = 500;
        private const int PacketSize = 64;
        private const int Address = 1;

        private const byte GetDescriptor = 0x06;
        private const byte SetAddress = 0x05;
        private const byte SetConfiguration = 0x09;
        private const byte SetLineCoding = 0x20;
        private const byte SetControlLineState = 0x22;

        private const int DeviceDescriptorType = 1;
        private const int ConfigDescriptorType = 2;
        private const byte InterfaceDescriptorType = 4;
        private const byte EndpointDescriptorType = 5;
        private const byte CdcCommClass = 0x02;
        private const byte CdcAbstractControl = 0x02;
        private const byte CdcDataClass = 0x0A;
        private const byte BulkTransfer = 0x02;

        private readonly IUsbCdcDevice device;
        private readonly List<byte> received = new List<byte>();

        private Step step;
        private bool active;
        private int waited, total, commInterface, bulkIn, bulkOut;
    }
}
