/* nRF52840, no SoftDevice: the application owns flash from 0x0. */
MEMORY {
  FLASH (rx) : ORIGIN = 0x00000000, LENGTH = 1024K
  RAM  (rwx) : ORIGIN = 0x20000000, LENGTH = 256K
}
_estack = ORIGIN(RAM) + LENGTH(RAM);
SECTIONS {
  .isr_vector : { KEEP(*(.isr_vector)) } > FLASH
  .text   : { *(.text*) *(.rodata*) } > FLASH
  /* C++ static constructors. Without these sections every file-scope object
     with a non-trivial constructor keeps the zeroed vtable pointer it got from
     .bss, and the first virtual call through it faults INVSTATE. */
  .preinit_array : { PROVIDE_HIDDEN(__preinit_array_start = .); KEEP(*(.preinit_array*)) PROVIDE_HIDDEN(__preinit_array_end = .); } > FLASH
  .init_array    : { PROVIDE_HIDDEN(__init_array_start = .);    KEEP(*(SORT(.init_array.*))) KEEP(*(.init_array*)) PROVIDE_HIDDEN(__init_array_end = .); } > FLASH
  .fini_array    : { PROVIDE_HIDDEN(__fini_array_start = .);    KEEP(*(SORT(.fini_array.*))) KEEP(*(.fini_array*)) PROVIDE_HIDDEN(__fini_array_end = .); } > FLASH
  /* Now that .init_array pulls in libstdc++ startup objects, the unwinder and
     __cxa_atexit come with them and want these symbols. */
  .ARM.exidx : { __exidx_start = .; *(.ARM.exidx*) __exidx_end = .; } > FLASH
  PROVIDE(__dso_handle = 0);
  _sidata = LOADADDR(.data);
  .data   : { _sdata = .; *(.data*) . = ALIGN(4); _edata = .; } > RAM AT> FLASH
  .bss    : { _sbss = .; *(.bss*) *(COMMON) . = ALIGN(4); _ebss = .; } > RAM
  ._heap  : { . = ALIGN(8); . = . + 32K; } > RAM
}
