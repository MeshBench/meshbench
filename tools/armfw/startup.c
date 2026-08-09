#include <stdint.h>
extern int main(void);
extern uint32_t _sidata, _sdata, _edata, _sbss, _ebss, _estack;

void Reset_Handler(void) {
  uint32_t *src = &_sidata, *dst = &_sdata;
  while (dst < &_edata) *dst++ = *src++;
  for (dst = &_sbss; dst < &_ebss;) *dst++ = 0;
  main();
  for (;;) {}
}
void Default_Handler(void) { for (;;) {} }

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
