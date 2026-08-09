#!/usr/bin/env bash
# Build a real MeshCore ESP32 firmware and boot it under Espressif's QEMU.
#
# Renode has no ESP32 platform, so the ESP32 half of the emulated backend uses
# Espressif's QEMU fork instead. Prebuilt binaries, no sudo:
#
#   gh release download <tag> --repo espressif/qemu \
#     --pattern 'qemu-xtensa-softmmu-*-x86_64-linux-gnu.tar.xz'
#
# It needs libslirp, which can be fetched from an Arch mirror and pointed at
# with LD_LIBRARY_PATH rather than installed system-wide.
set -euo pipefail

MC=${MESHCORE:-$HOME/msim/MeshCore}
QEMU=${ESPQEMU:-$HOME/msim/espqemu}
ENVNAME=${1:-Heltec_v2_repeater}
CHIP=${2:-esp32}
BOOT_OFF=${3:-0x1000}     # esp32 boots at 0x1000; esp32s3 at 0x0
FREQ=${4:-40m}

cd "$MC"
pio run -e "$ENVNAME"
B=".pio/build/$ENVNAME"
ET="$HOME/.platformio/packages/tool-esptoolpy/esptool.py"
IMG=/tmp/${CHIP}-${ENVNAME}.bin

# Flash size must match the image header or ESP-IDF asserts in do_core_init:
#   "Detected size(4096k) smaller than the size in the binary image header(8192k)"
python3 "$ET" --chip "$CHIP" merge_bin -o "$IMG" \
  --flash_mode dio --flash_freq "$FREQ" --flash_size 8MB \
  "$BOOT_OFF" "$B/bootloader.bin" 0x8000 "$B/partitions.bin" 0x10000 "$B/firmware.bin"

python3 - "$IMG" <<'PY'
import sys
p = sys.argv[1]
d = open(p, 'rb').read()
target = 8 * 1024 * 1024          # QEMU accepts only 2/4/8/16 MB images
open(p, 'wb').write(d + b'\xff' * (target - len(d)))
print(f"  padded to {target} bytes")
PY

export LD_LIBRARY_PATH="$QEMU/lib:${LD_LIBRARY_PATH:-}"
exec "$QEMU/bin/qemu-system-xtensa" -nographic -machine "$CHIP" \
  -drive file="$IMG",if=mtd,format=raw -serial mon:stdio
