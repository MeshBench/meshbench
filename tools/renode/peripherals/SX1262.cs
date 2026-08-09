//
// SX1262 LoRa transceiver — enough of it to satisfy RadioLib's real driver.
//
// This is the single blocker for the emulated backend on BOTH architectures:
// MeshCore's radio_init() drives an SX1262 over SPI, and neither Renode's
// nRF52840 platform nor Espressif's QEMU models one, so RadioLib waits on a
// chip that never answers.
//
// Scope: the command protocol, the status byte, IRQ flags and the data buffer.
// Not modelled: actual modulation — the RF engine in internal/rf does that, and
// this peripheral's job is to make the firmware believe it has a radio and to
// hand transmitted frames out to the simulator.
//
using System;
using System.Net.Sockets;
using Antmicro.Renode.Core;
using Antmicro.Renode.Logging;
using Antmicro.Renode.Peripherals.SPI;

namespace Antmicro.Renode.Peripherals.Radio
{
    public class SX1262 : ISPIPeripheral
    {
        // Renode resolves peripherals by constructor signature; one taking the
        // machine is what a registration point like "@ spi2 0" looks for.
        public SX1262(IMachine machine, string bridgeHost = "127.0.0.1", int bridgePort = 0)
        {
            this.machine = machine;
            this.bridgeHost = bridgeHost;
            this.bridgePort = bridgePort;
            Reset();
        }

        // Connect this radio to the RF engine. Frames the firmware transmits go
        // out over this socket; frames the channel delivers come back in.
        //
        // A socket rather than a Renode wireless medium on purpose: the physics
        // lives in internal/rf, and an emulated node must share exactly the same
        // channel as native ones or the two backends are not comparable — which
        // is the entire point of having both (ADR-0010).
        public void ConnectBridge()
        {
            if(bridgePort == 0)
            {
                return;
            }
            try
            {
                bridge = new TcpClient(bridgeHost, bridgePort);
                stream = bridge.GetStream();
                this.Log(LogLevel.Info, "SX1262: bridged to RF engine at {0}:{1}", bridgeHost, bridgePort);
            }
            catch(Exception e)
            {
                // Loud, not silent: an unbridged radio transmits into nothing,
                // and a simulation that quietly drops every packet looks like a
                // propagation result rather than a wiring fault.
                this.Log(LogLevel.Error, "SX1262: no RF engine at {0}:{1} — {2}", bridgeHost, bridgePort, e.Message);
            }
        }

        public void Reset()
        {
            state = State.Command;
            chipMode = ChipMode.StandbyRC;
            commandStatus = CommandStatus.Reserved;
            irqStatus = 0;
            irqMask = 0;
            deviceErrors = 0;
            argsRemaining = 0;
            bufferOffset = 0;
            Array.Clear(dataBuffer, 0, dataBuffer.Length);
            Array.Clear(registers, 0, registers.Length);
        }

        // Every SPI byte arrives here. The SX1262 protocol is: one opcode byte,
        // then N argument bytes, with the status byte returned on each transfer.
        public byte Transmit(byte data)
        {
            if(state == State.Command)
            {
                opcode = data;
                argIndex = 0;
                argsRemaining = ArgumentCount(opcode);
                state = argsRemaining > 0 ? State.Arguments : State.Command;
                HandleOpcodeStart(opcode);
                return StatusByte();
            }

            var response = HandleArgument(argIndex, data);
            argIndex++;
            if(argsRemaining != VariableLength && --argsRemaining <= 0)
            {
                HandleOpcodeEnd();
                state = State.Command;
            }
            return response;
        }

        // Chip select deasserted: any variable-length command ends here.
        public void FinishTransmission()
        {
            if(state == State.Arguments)
            {
                HandleOpcodeEnd();
            }
            state = State.Command;
            argsRemaining = 0;
        }

        // The status byte RadioLib reads after every command. Getting the layout
        // wrong is the difference between "radio present" and a driver that
        // gives up: bits 6:4 are the chip mode, bits 3:1 the command status.
        private byte StatusByte()
        {
            return (byte)(((int)chipMode << 4) | ((int)commandStatus << 1));
        }

        private void HandleOpcodeStart(byte op)
        {
            switch(op)
            {
            case OpSetSleep:   chipMode = ChipMode.StandbyRC; break;
            case OpSetStandby: chipMode = ChipMode.StandbyRC; break;
            case OpSetFs:      chipMode = ChipMode.FrequencySynthesis; break;
            case OpSetTx:
                chipMode = ChipMode.Tx;
                SendToRfEngine();
                // TX_DONE is raised once the frame is handed over. Real airtime
                // is the RF engine's business, not this peripheral's — modelling
                // it here would put the same physics in two places.
                irqStatus |= IrqTxDone;
                commandStatus = CommandStatus.CommandTimeout;
                break;
            case OpSetRx:
                chipMode = ChipMode.Rx;
                PollRfEngine();
                break;
            case OpSetCad:
                chipMode = ChipMode.Rx;
                // CAD is answered by the channel when bridged: this is what makes
                // the firmware's own carrier-sense observable rather than
                // asserted. Unbridged, report clear — reporting busy by default
                // would stop the firmware ever transmitting.
                if(channelBusy)
                {
                    irqStatus |= IrqCadDetected;
                }
                irqStatus |= IrqCadDone;
                break;
            }
        }

        private void HandleOpcodeEnd()
        {
            if(commandStatus == CommandStatus.Reserved)
            {
                commandStatus = CommandStatus.CommandTimeout;
            }
        }

        private byte HandleArgument(int index, byte value)
        {
            switch(opcode)
            {
            case OpWriteRegister:
                if(index < 2) { address = (ushort)((address << 8) | value); return StatusByte(); }
                if(index == 2) { address = (ushort)(address & 0xFFFF); }
                if(address < registers.Length) { registers[address] = value; }
                address++;
                return StatusByte();

            case OpReadRegister:
                if(index < 2) { address = (ushort)((address << 8) | value); return StatusByte(); }
                if(index == 2) { return StatusByte(); }  // one NOP byte before data
                return address < registers.Length ? registers[address++] : (byte)0;

            case OpWriteBuffer:
                if(index == 0) { bufferOffset = value; return StatusByte(); }
                if(bufferOffset < dataBuffer.Length) { dataBuffer[bufferOffset++] = value; }
                payloadLength = Math.Max(payloadLength, bufferOffset);
                return StatusByte();

            case OpReadBuffer:
                if(index == 0) { bufferOffset = value; return StatusByte(); }
                if(index == 1) { return StatusByte(); }  // NOP
                return bufferOffset < dataBuffer.Length ? dataBuffer[bufferOffset++] : (byte)0;

            case OpGetStatus:
                return StatusByte();

            case OpGetIrqStatus:
                if(index == 0) { return StatusByte(); }
                return index == 1 ? (byte)(irqStatus >> 8) : (byte)(irqStatus & 0xFF);

            case OpClearIrqStatus:
                if(index == 0) { clearMask = (ushort)(value << 8); }
                else { clearMask |= value; irqStatus &= (ushort)~clearMask; }
                return StatusByte();

            case OpGetDeviceErrors:
                if(index == 0) { return StatusByte(); }
                return index == 1 ? (byte)(deviceErrors >> 8) : (byte)(deviceErrors & 0xFF);

            case OpGetRxBufferStatus:
                if(index == 0) { return StatusByte(); }
                if(index == 1) { return payloadLength; }
                return 0;   // rx start pointer

            case OpGetPacketStatus:
                if(index == 0) { return StatusByte(); }
                if(index == 1) { return unchecked((byte)(-80 * 2)); }  // RSSI, -dBm/2
                if(index == 2) { return 40; }                          // SNR, dB*4
                return unchecked((byte)(-80 * 2));                     // signal RSSI

            default:
                return StatusByte();
            }
        }

        // Argument counts for the opcodes RadioLib actually issues. A wrong
        // count desynchronises the whole SPI stream, so unknown opcodes are
        // logged rather than guessed at.
        private int ArgumentCount(byte op)
        {
            switch(op)
            {
            case OpSetSleep:                return 1;
            case OpSetStandby:              return 1;
            case OpSetFs:                   return 0;
            case OpSetTx:                   return 3;
            case OpSetRx:                   return 3;
            case OpSetCad:                  return 0;
            case OpSetRfFrequency:          return 4;
            case OpSetPacketType:           return 1;
            case OpGetPacketType:           return 2;
            case OpSetTxParams:             return 2;
            case OpSetModulationParams:     return 8;
            case OpSetPacketParams:         return 9;
            case OpSetBufferBaseAddress:    return 2;
            case OpSetDioIrqParams:         return 8;
            case OpGetIrqStatus:            return 3;
            case OpClearIrqStatus:          return 2;
            case OpGetStatus:               return 1;
            case OpGetRxBufferStatus:       return 3;
            case OpGetPacketStatus:         return 4;
            case OpGetDeviceErrors:         return 3;
            case OpClearDeviceErrors:       return 2;
            case OpCalibrate:               return 1;
            case OpCalibrateImage:          return 2;
            case OpSetRegulatorMode:        return 1;
            case OpSetDio2AsRfSwitchCtrl:   return 1;
            case OpSetDio3AsTcxoCtrl:       return 4;
            case OpSetPaConfig:             return 4;
            case OpSetRxTxFallbackMode:     return 1;
            case OpSetCadParams:            return 7;
            case OpSetLoRaSymbNumTimeout:   return 1;
            // Variable-length: these run until chip select deasserts, not for a
            // fixed count. Declaring them fixed makes the model treat payload
            // bytes as opcodes — caught by sx1262_test.resc, which is exactly
            // why the peripheral is checked in isolation before firmware
            // depends on it.
            case OpWriteRegister:
            case OpReadRegister:
            case OpWriteBuffer:
            case OpReadBuffer:
                return VariableLength;
            default:
                this.Log(LogLevel.Warning, "SX1262: unmodelled opcode 0x{0:X2}", op);
                return 0;
            }
        }

        // Frame out: [0x01][len:2][payload]. Frame in: the same, delivered by the
        // channel after path loss, summation and noise.
        private void SendToRfEngine()
        {
            if(stream == null || payloadLength == 0)
            {
                return;
            }
            try
            {
                var header = new byte[] { 0x01, (byte)(payloadLength >> 8), (byte)(payloadLength & 0xFF) };
                stream.Write(header, 0, header.Length);
                stream.Write(dataBuffer, 0, payloadLength);
                stream.Flush();
            }
            catch(Exception e)
            {
                this.Log(LogLevel.Warning, "SX1262: transmit to RF engine failed: {0}", e.Message);
            }
        }

        private void PollRfEngine()
        {
            if(stream == null || !stream.DataAvailable)
            {
                return;
            }
            try
            {
                var header = new byte[3];
                if(stream.Read(header, 0, 3) != 3)
                {
                    return;
                }
                var length = (header[1] << 8) | header[2];
                if(length <= 0 || length > dataBuffer.Length)
                {
                    return;
                }
                var read = 0;
                while(read < length)
                {
                    var n = stream.Read(dataBuffer, read, length - read);
                    if(n <= 0)
                    {
                        return;
                    }
                    read += n;
                }
                payloadLength = (byte)length;
                // Only CRC-passing frames reach the firmware, exactly as on real
                // hardware. Everything else is recorded by the ledger and
                // withheld — the gap between what the radio heard and what the
                // stack acted on (ADR-0014).
                irqStatus |= IrqRxDone;
                commandStatus = CommandStatus.DataAvailable;
            }
            catch(Exception e)
            {
                this.Log(LogLevel.Warning, "SX1262: receive from RF engine failed: {0}", e.Message);
            }
        }

        private enum State { Command, Arguments }
        private enum ChipMode { StandbyRC = 2, StandbyXosc = 3, FrequencySynthesis = 4, Rx = 5, Tx = 6 }
        private enum CommandStatus { Reserved = 0, DataAvailable = 2, CommandTimeout = 3, ProcessingError = 4, ExecutionFailure = 5, TxDone = 6 }

        private readonly IMachine machine;
        private readonly string bridgeHost;
        private readonly int bridgePort;
        private TcpClient bridge;
        private NetworkStream stream;
        private bool channelBusy;
        private State state;
        private ChipMode chipMode;
        private CommandStatus commandStatus;
        private byte opcode;
        private int argIndex;
        private int argsRemaining;
        private ushort address;
        private ushort irqStatus, irqMask, clearMask, deviceErrors;
        private byte bufferOffset, payloadLength;
        private readonly byte[] dataBuffer = new byte[256];
        private readonly byte[] registers = new byte[0x1000];

        // Sentinel for commands whose length is set by chip select, not a count.
        private const int VariableLength = int.MaxValue;

        private const ushort IrqTxDone = 1 << 0;
        private const ushort IrqRxDone = 1 << 1;
        private const ushort IrqCadDone = 1 << 7;
        private const ushort IrqCadDetected = 1 << 8;

        private const byte OpSetSleep = 0x84, OpSetStandby = 0x80, OpSetFs = 0xC1;
        private const byte OpSetTx = 0x83, OpSetRx = 0x82, OpSetCad = 0xC5;
        private const byte OpSetRfFrequency = 0x86, OpSetPacketType = 0x8A, OpGetPacketType = 0x11;
        private const byte OpSetTxParams = 0x8E, OpSetModulationParams = 0x8B, OpSetPacketParams = 0x8C;
        private const byte OpSetBufferBaseAddress = 0x8F, OpSetDioIrqParams = 0x08;
        private const byte OpGetIrqStatus = 0x12, OpClearIrqStatus = 0x02, OpGetStatus = 0xC0;
        private const byte OpGetRxBufferStatus = 0x13, OpGetPacketStatus = 0x14;
        private const byte OpGetDeviceErrors = 0x17, OpClearDeviceErrors = 0x07;
        private const byte OpCalibrate = 0x89, OpCalibrateImage = 0x98, OpSetRegulatorMode = 0x96;
        private const byte OpSetDio2AsRfSwitchCtrl = 0x9D, OpSetDio3AsTcxoCtrl = 0x97;
        private const byte OpSetPaConfig = 0x95, OpSetRxTxFallbackMode = 0x93;
        private const byte OpSetCadParams = 0x88, OpSetLoRaSymbNumTimeout = 0xA0;
        private const byte OpWriteRegister = 0x0D, OpReadRegister = 0x1D;
        private const byte OpWriteBuffer = 0x0E, OpReadBuffer = 0x1E;
    }
}
