#include <Mesh.h>
#include <Dispatcher.h>
#include <Utils.h>
#include <vector>

using namespace mesh;

// nRF52 UART0, driven as a UARTE: the EasyDMA half, not the legacy registers.
// Register map from the nRF52840 product spec.
//
// The distinction decides whether anything can hear this board. Renode's
// NRF52840_UART accepts a write to the legacy TXD register and logs it as
// unhandled, so output that way appears in the emulator's own log and reaches
// no terminal, no file backend and no socket - a board that could be read over
// the emulator's shoulder and never talked to. The EasyDMA path is the one the
// model implements, and it is what the Arduino cores drive on this part anyway.
#define UART_BASE      0x40002000u
#define TASKS_STARTRX  0x000
#define TASKS_STARTTX  0x008
#define EVENTS_ENDRX   0x110
#define EVENTS_ENDTX   0x120
#define UART_ENABLE    0x500
#define PSEL_TXD       0x50C
#define PSEL_RXD       0x514
#define UART_BAUDRATE  0x524
#define RXD_PTR        0x534
#define RXD_MAXCNT     0x538
#define TXD_PTR        0x544
#define TXD_MAXCNT     0x548
#define REG(o) (*(volatile uint32_t*)(UART_BASE + (o)))

// EasyDMA reads and writes memory rather than a register, so the two ends of
// the port each need a byte of RAM to point at.
static volatile uint8_t uart_tx_byte;
static volatile uint8_t uart_rx_byte;

static void uart_rx_arm() {
  REG(RXD_PTR)       = (uint32_t)(uintptr_t)&uart_rx_byte;
  REG(RXD_MAXCNT)    = 1;
  REG(EVENTS_ENDRX)  = 0;
  REG(TASKS_STARTRX) = 1;
}

static void uart_init() {
  REG(PSEL_TXD)      = 6;           // P0.06
  REG(PSEL_RXD)      = 8;           // P0.08
  REG(UART_BAUDRATE) = 0x01D7E000;  // 115200
  REG(UART_ENABLE)   = 8;           // UARTE enabled
  uart_rx_arm();
}

// -1 when nothing has arrived. Polled rather than interrupt-driven: this
// firmware has no scheduler to be woken, and the console is the last thing it
// does.
static int uart_getc() {
  if (!REG(EVENTS_ENDRX)) return -1;
  int c = (int)uart_rx_byte;
  uart_rx_arm();
  return c;
}

static void uart_putc(char c) {
  uart_tx_byte       = (uint8_t)c;
  REG(TXD_PTR)       = (uint32_t)(uintptr_t)&uart_tx_byte;
  REG(TXD_MAXCNT)    = 1;
  REG(EVENTS_ENDTX)  = 0;
  REG(TASKS_STARTTX) = 1;
  // Bounded, because a transfer that never ends must not take the whole run
  // with it: a ceiling of 100,000 spins per character once burned 45 s of
  // emulated time over one line and read as a hang inside Mesh::begin().
  for (int i = 0; i < 1000 && !REG(EVENTS_ENDTX); i++) {}
  REG(EVENTS_ENDTX) = 0;
}

static void uart_puts(const char* s) { while (*s) uart_putc(*s++); }

// nRF52840 SPIM2 at 0x40023000 — the controller Renode models and the RAK4631
// wires the SX1262 to. Raw register poking rather than a driver: this firmware
// exists to prove the chain firmware -> SPI -> SX1262 model -> bridge -> RF
// engine, and a driver would only add a layer to be wrong in.
#define SPI_BASE 0x40023000u
#define SPI_REG(o) (*(volatile uint32_t*)(SPI_BASE + (o)))

// nRF52 legacy SPI. Renode's NRF52840_SPI models this, not SPIM: an EasyDMA
// attempt produced 22 unhandled accesses (TASKS_START, EVENTS_END, RXD.PTR,
// TXD.PTR all unimplemented) and the transaction never left the controller.
//
// Note the offsets: RXD is 0x518 and TXD is 0x51C. Having them the wrong way
// round is why the first attempt read back 0x00 — it was writing to the receive
// register and reading from the transmit one.
static uint8_t spi_rx[64];

#define SPI_EVENTS_READY 0x108
#define SPI_ENABLE       0x500
#define SPI_PSEL_SCK     0x508
#define SPI_PSEL_MOSI    0x50C
#define SPI_PSEL_MISO    0x510
#define SPI_RXD          0x518
#define SPI_TXD          0x51C
#define SPI_FREQUENCY    0x524

static void spi_init() {
  SPI_REG(SPI_PSEL_SCK)  = 19;
  SPI_REG(SPI_PSEL_MOSI) = 20;
  SPI_REG(SPI_PSEL_MISO) = 21;
  SPI_REG(SPI_FREQUENCY) = 0x80000000;  // 8 Mbps
  SPI_REG(SPI_ENABLE)    = 1;           // legacy SPI
}

static uint8_t spi_xfer(uint8_t out) {
  SPI_REG(SPI_EVENTS_READY) = 0;
  SPI_REG(SPI_TXD) = out;
  for (int i = 0; i < 10000 && !SPI_REG(SPI_EVENTS_READY); i++) {}
  SPI_REG(SPI_EVENTS_READY) = 0;
  return (uint8_t)SPI_REG(SPI_RXD);
}

static void spi_transfer(const uint8_t* out, int n) {
  for (int i = 0; i < n; i++) spi_rx[i] = spi_xfer(out[i]);
}

static uint8_t sx_cmd(uint8_t opcode, const uint8_t* args, int n) {
  uint8_t buf[32];
  buf[0] = opcode;
  for (int i = 0; i < n && i < 31; i++) buf[i + 1] = args[i];
  spi_transfer(buf, n + 1);
  return spi_rx[0];
}

class SimRadio : public Radio {
public:
  volatile int txCount = 0;
  bool startSendRaw(const uint8_t* bytes, int len) override {
    txCount++;
    // WriteBuffer at offset 0, then SetTx. The SX1262 model forwards the buffer
    // to the RF engine over its bridge, so this frame joins the same channel a
    // native node's would.
    uint8_t wb[64];
    wb[0] = 0x0E;                         // WriteBuffer
    wb[1] = 0x00;                         // offset
    int n = len < 62 ? len : 62;
    for (int i = 0; i < n; i++) wb[i + 2] = bytes[i];
    spi_transfer(wb, n + 2);
    const uint8_t txArgs[3] = {0, 0, 0};  // timeout: no timeout
    sx_cmd(0x83, txArgs, 3);              // SetTx
    return true;
  }
  bool isSendComplete() override { return true; }
  void onSendFinished() override {}
  int recvRaw(uint8_t*, int) override { return 0; }
  uint32_t getEstAirtimeFor(int len) override { return (uint32_t)(len * 8 * 1000 / 1758); }
  float packetScore(float snr, int len) override { return snr * 100 - len; }
  int getNoiseFloor() const override { return -117; }
  bool isInRecvMode() const override { return true; }
};
class SimClock : public MillisecondClock {
public: unsigned long now = 0; unsigned long getMillis() override { return now; }
};
class SimRTC : public RTCClock {
public: uint32_t t = 1754700000;
  uint32_t getCurrentTime() override { return t; }
  void setCurrentTime(uint32_t v) override { t = v; }
};
class SimRNG : public RNG {
public: uint64_t s = 4417;
  void random(uint8_t* d, size_t n) override {
    for (size_t i = 0; i < n; i++) { s = s*6364136223846793005ULL + 1442695040888963407ULL; d[i] = (uint8_t)(s>>33); }
  }
};
class SimTables : public MeshTables {
public:
  bool wasSeen(const Packet*) override { return false; }
  void markSeen(const Packet*) override {}
  void clear(const Packet*) override {}
};
static Packet g_pool[16];
class SimPM : public PacketManager {
public:
  SimPM() { for (int i = 0; i < 16; i++) free_list[i] = &g_pool[i]; n = 16; }
  Packet* allocNew() override { return n ? free_list[--n] : nullptr; }
  void free(Packet* p) override { if (p && n < 16) free_list[n++] = p; }
  void queueOutbound(Packet* p, uint8_t, uint32_t w) override { if (on < 8) { out[on] = p; ow[on] = w; on++; } }
  Packet* getNextOutbound(uint32_t now) override {
    for (int i = 0; i < on; i++) if (ow[i] <= now) { Packet* p = out[i]; for (int j=i;j<on-1;j++){out[j]=out[j+1];ow[j]=ow[j+1];} on--; return p; }
    return nullptr;
  }
  int getOutboundCount(uint32_t now) const override { int c=0; for (int i=0;i<on;i++) if (ow[i]<=now) c++; return c; }
  int getOutboundTotal() const override { return on; }
  int getFreeCount() const override { return n; }
  Packet* getOutboundByIdx(int i) override { return (i>=0&&i<on)?out[i]:nullptr; }
  Packet* removeOutboundByIdx(int i) override {
    if (i<0||i>=on) return nullptr; Packet* p=out[i];
    for (int j=i;j<on-1;j++){out[j]=out[j+1];ow[j]=ow[j+1];} on--; return p;
  }
  void queueInbound(Packet* p, uint32_t w) override { if (in_n<8){inb[in_n]=p;iw[in_n]=w;in_n++;} }
  Packet* getNextInbound(uint32_t now) override {
    for (int i=0;i<in_n;i++) if (iw[i]<=now) { Packet* p=inb[i]; for(int j=i;j<in_n-1;j++){inb[j]=inb[j+1];iw[j]=iw[j+1];} in_n--; return p; }
    return nullptr;
  }
private:
  Packet* free_list[16]; int n = 0;
  Packet* out[8]; uint32_t ow[8]; int on = 0;
  Packet* inb[8]; uint32_t iw[8]; int in_n = 0;
};

// Mesh's constructor is protected, so a node is a subclass — the same shape the
// host harness uses, and where the log hooks would be overridden.
class SimNode : public Mesh {
public:
  SimNode(SimRadio& r, SimClock& c, SimRNG& g, SimRTC& t, SimPM& m, SimTables& tb)
      : Mesh(r, c, g, t, m, tb) {}
};

static SimRadio radio; static SimClock clk; static SimRNG rng;
static SimRTC rtc; static SimPM mgr; static SimTables tables;

static void console(const SimRadio& radio);

int main() {
  uart_init();
  spi_init();
  uart_puts("MSIM bare-metal nRF52840, no SoftDevice\r\n");

  // Talk to the radio before the mesh does, so a wiring fault is reported here
  // rather than as an inexplicable mesh failure later.
  // One NOP byte, not a null pointer: sx_cmd dereferences args[0], and passing
  // null with n=1 is undefined behaviour. GCC is entitled to delete everything
  // after it — and did, silently reducing main() to 72 bytes with the whole
  // mesh stack garbage-collected out of the image.
  const uint8_t nop = 0x00;
  uint8_t st = sx_cmd(0xC0, &nop, 1);
  uart_puts("SX1262 GetStatus: ");
  const char* hex = "0123456789ABCDEF";
  uart_putc(hex[(st >> 4) & 0xF]); uart_putc(hex[st & 0xF]);
  uart_puts("\r\n");

  static SimNode node(radio, clk, rng, rtc, mgr, tables);
  node.begin();
  uart_puts("Mesh::begin() ok\r\n");

  for (int i = 0; i < 200; i++) {
    clk.now += 10;
    node.loop();
    if (i % 50 == 0) uart_putc('.');   // progress, so slow is distinguishable from stuck
  }
  uart_puts("\r\nloop x200 ok\r\n");

  Packet* p = mgr.allocNew();
  if (p) {
    p->header = 0x12; p->payload_len = 8;
    for (int i = 0; i < 8; i++) p->payload[i] = (uint8_t)(0xA0 + i);
    node.sendFlood(p);
    uart_puts("sendFlood issued\r\n");
    for (int i = 0; i < 400; i++) { clk.now += 10; node.loop(); }
  }
  uart_puts(radio.txCount > 0 ? "TX OK - mesh stack ran on ARM\r\n" : "no TX\r\n");
  console(radio);
}

// The console, which is the only way to ask this board anything once it has
// finished its own checks.
//
// It is here because the emulator's serial port is two-way and nothing proved
// the inbound half: output alone is satisfied by a board printing into a file,
// which is what this used to do. A board that answers has been typed at, and
// there is no other way to be sure.
static void console(const SimRadio& radio) {
  uart_puts("ready\r\n");
  char line[32];
  int n = 0;
  for (;;) {
    int c = uart_getc();
    if (c < 0) continue;
    if (c == '\r' || c == '\n') {
      // A terminal sends both, so an empty line here is the second half of the
      // one just answered rather than somebody pressing return at nothing.
      if (n == 0) continue;
      line[n] = 0;
      uart_puts("\r\n-> ");
      if (line[0] == 'v') {
        uart_puts("MSIM bare-metal nRF52840");
      } else if (line[0] == 't') {
        uart_puts("tx=");
        uart_putc((char)('0' + (radio.txCount % 10)));
      } else {
        uart_puts("unknown: ");
        uart_puts(line);
      }
      uart_puts("\r\n");
      n = 0;
      continue;
    }
    if (n < (int)sizeof(line) - 1) line[n++] = (char)c;
    uart_putc((char)c);  // echoed, so a person typing sees what they typed
  }
}
