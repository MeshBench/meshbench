#include <stdint.h>
extern int main(void);
extern void __libc_init_array(void);
// newlib's __libc_init_array calls _init/_fini, which live in crti.o/crtn.o —
// objects -nostartfiles deliberately leaves out. Nothing here needs them.
void _init(void) {}
void _fini(void) {}
extern uint32_t _sidata, _sdata, _edata, _sbss, _ebss, _estack;

void Reset_Handler(void) {
  // Enable the FPU before anything else. The Cortex-M4F powers up with CP10 and
  // CP11 disabled, so the first floating-point instruction takes a UsageFault —
  // which lands in Default_Handler's infinite loop and looks exactly like a hang
  // deep inside application code.
  //
  // Dispatcher::begin() computes the duty-cycle budget in float, so MeshCore
  // faults on its very first call. Compiling with -mfpu enables the codegen; it
  // does not enable the hardware.
  *(volatile uint32_t *)0xE000ED88 |= (0xFu << 20);  // CPACR: full access CP10/CP11
  __asm volatile("dsb");
  __asm volatile("isb");

  uint32_t *src = &_sidata, *dst = &_sdata;
  while (dst < &_edata) *dst++ = *src++;
  for (dst = &_sbss; dst < &_ebss;) *dst++ = 0;
  // Run the C++ static constructors. Skipping this does not fail to link and
  // does not crash at startup: the objects simply keep null vtable pointers,
  // and the program dies much later at the first virtual call through one.
  __libc_init_array();
  main();
  for (;;) {}
}
// A fault used to be indistinguishable from a hang: both ended in this
// function's infinite loop, with no output. Reporting IPSR and the faulting PC
// turns "it stopped somewhere" into an address you can pass to addr2line.
// Pokes UART0's legacy TXD directly rather than calling the C++ uart_putc: a
// fault handler must not depend on the rest of the program still being sane,
// and it must link even if that symbol is mangled or garbage-collected. The
// emulator does not model that register, so a fault is reported in its own log
// as an unhandled write rather than on the console - which is the right way
// round for a report that must survive the console itself being wedged.
static void uart_putc(char c) {
  *(volatile unsigned int*)0x4000251C = (unsigned int)(unsigned char)c;
  for (volatile int i = 0; i < 50; i++) {}
}
static void put_hex32(unsigned int v) {
  const char* h = "0123456789ABCDEF";
  for (int i = 28; i >= 0; i -= 4) uart_putc(h[(v >> i) & 0xF]);
}
void Fault_Report(unsigned int* frame) {
  unsigned int ipsr; __asm volatile("mrs %0, ipsr" : "=r"(ipsr));
  const char* m = "\r\nFAULT vec=";
  while (*m) uart_putc(*m++);
  put_hex32(ipsr);
  m = " pc="; while (*m) uart_putc(*m++);
  put_hex32(frame[6]);                                  // stacked PC
  m = " cfsr="; while (*m) uart_putc(*m++);
  put_hex32(*(volatile unsigned int*)0xE000ED28);       // CFSR
  uart_putc('\r'); uart_putc('\n');
  for (;;) {}
}
// Naked so the exception frame pointer is whichever stack was in use; the EXC_RETURN
// bit 2 selects MSP vs PSP.
__attribute__((naked)) void Default_Handler(void) {
  __asm volatile(
    "tst lr, #4\n"
    "ite eq\n"
    "mrseq r0, msp\n"
    "mrsne r0, psp\n"
    "b Fault_Report\n");
}

// newlib wants _sbrk for malloc. Hand it the region the linker reserved after
// .bss — a bare-metal image has no OS to ask.
extern uint32_t _ebss;
void *_sbrk(int incr) {
  static char *heap = 0;
  extern uint32_t _estack;
  if (!heap) heap = (char *)&_ebss;
  char *prev = heap;
  if (heap + incr > (char *)&_estack - 4096) return (void *)-1;  // keep stack room
  heap += incr;
  return prev;
}
int _close(int f) { (void)f; return -1; }
int _fstat(int f, void *st) { (void)f; (void)st; return 0; }
int _isatty(int f) { (void)f; return 1; }
int _lseek(int f, int o, int w) { (void)f; (void)o; (void)w; return 0; }
int _read(int f, char *b, int n) { (void)f; (void)b; (void)n; return 0; }
int _write(int f, const char *b, int n) { (void)f; (void)b; return n; }
void _exit(int c) { (void)c; for (;;) {} }
int _kill(int p, int s) { (void)p; (void)s; return -1; }
int _getpid(void) { return 1; }

__attribute__((section(".isr_vector"), used))
void (* const g_vectors[])(void) = {
  (void (*)(void))&_estack,
  Reset_Handler,
  Default_Handler, Default_Handler, Default_Handler, Default_Handler,
  Default_Handler, 0, 0, 0, 0,
  Default_Handler, Default_Handler, 0, Default_Handler, Default_Handler,
};
