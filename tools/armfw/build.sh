#!/usr/bin/env bash
# Cross-compile MeshCore for nRF52840 with NO SoftDevice, linked at 0x0.
#
# This is what makes the Renode backend work. The published .uf2 is linked above
# a SoftDevice (0x26000 for s140 v6.1.1) and makes 119 SVC calls into it — none
# of which the repeater needs, because examples/simple_repeater/main.cpp
# contains no BLE code at all. The SoftDevice is imposed by the Adafruit nRF52
# Arduino core's linker layout, not by anything MeshCore calls.
#
# Building it ourselves removes the SoftDevice entirely: 119 SVC sites -> 5
# (residual data patterns, not instructions), and the image boots directly.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)

MC=${MESHCORE:-$HOME/msim/MeshCore}
CRY=${CRYPTO:-$HOME/msim/arduinolibs/libraries/Crypto}
# Absolute, because the compile runs from build/ - a relative shim path
# resolved against the wrong directory there and Stream.h went missing.
SHIM=${SHIM:-$HERE/../../internal/mesh/shim}
TC=$(ls -d "$HOME"/.platformio/packages/toolchain-gccarmnoneeabi/bin 2>/dev/null || true)
[ -n "$TC" ] && export PATH="$TC:$PATH"

FLAGS="-mcpu=cortex-m4 -mthumb -mfloat-abi=softfp -mfpu=fpv4-sp-d16 -Os
       -ffunction-sections -fdata-sections -fno-exceptions -fno-rtti
       -fno-threadsafe-statics"
INC="-I $MC/src -I $SHIM -I $CRY -I $MC/lib/ed25519"

rm -rf "$HERE/build" && mkdir -p "$HERE/build" && cd "$HERE/build"
arm-none-eabi-g++ -std=c++17 $FLAGS $INC -c "$MC"/src/{Utils,Packet,Identity,Mesh,Dispatcher}.cpp
arm-none-eabi-g++ -std=c++17 $FLAGS $INC -c \
  "$CRY"/{SHA256,AES128,AESCommon,BlockCipher,Crypto,Ed25519,BigNumberUtil,Curve25519,SHA512,Hash}.cpp
arm-none-eabi-gcc -std=c11 $FLAGS -I "$MC/lib/ed25519" -c "$MC"/lib/ed25519/*.c
arm-none-eabi-g++ -std=c++17 $FLAGS $INC -c "$SHIM/HostRNG.cpp" "$HERE/main.cpp"
arm-none-eabi-gcc $FLAGS -c "$HERE/startup.c"
arm-none-eabi-g++ $FLAGS -T "$HERE/link.ld" -nostartfiles -Wl,--gc-sections \
  -o fw.elf ./*.o -lc -lm -lnosys
arm-none-eabi-objcopy -O binary fw.elf fw.bin
arm-none-eabi-size fw.elf
python3 - <<'PY'
import struct
d = open('fw.bin', 'rb').read()
sp, pc = struct.unpack_from('<II', d)
svc = sum(1 for i in range(0, len(d) - 1, 2)
          if struct.unpack_from('<H', d, i)[0] & 0xFF00 == 0xDF00)
print(f"  fw.bin {len(d)} bytes  SP=0x{sp:08X} reset=0x{pc:08X}  SVC halfwords: {svc}")
PY
