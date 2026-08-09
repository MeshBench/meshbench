#include <Mesh.h>
#include <Dispatcher.h>
#include <Utils.h>
#include <vector>

using namespace mesh;

// nRF52 UART0 (legacy UART, which is what Renode's NRF52840_UART models).
// Register map from the nRF52840 product spec.
#define UART_BASE      0x40002000u
#define TASKS_STARTTX  0x008
#define EVENTS_TXDRDY  0x11C
#define UART_ENABLE    0x500
#define PSEL_TXD       0x50C
#define UART_TXD       0x51C
#define UART_BAUDRATE  0x524
#define REG(o) (*(volatile uint32_t*)(UART_BASE + (o)))

static void uart_init() {
  REG(PSEL_TXD)      = 6;           // P0.06
  REG(UART_BAUDRATE) = 0x01D7E000;  // 115200
  REG(UART_ENABLE)   = 4;           // UART enabled
  REG(TASKS_STARTTX) = 1;
}

static void uart_putc(char c) {
  REG(EVENTS_TXDRDY) = 0;
  REG(UART_TXD) = (uint32_t)(uint8_t)c;
  // Bounded wait: a spin with no ceiling would hang the whole image if the
  // peripheral is unmodelled, which is exactly the failure we are hunting.
  for (int i = 0; i < 100000 && !REG(EVENTS_TXDRDY); i++) {}
  REG(EVENTS_TXDRDY) = 0;
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

  for (int i = 0; i < 200; i++) { clk.now += 10; node.loop(); }

  Packet* p = mgr.allocNew();
  if (p) {
    p->header = 0x12; p->payload_len = 8;
    for (int i = 0; i < 8; i++) p->payload[i] = (uint8_t)(0xA0 + i);
    node.sendFlood(p);
    for (int i = 0; i < 400; i++) { clk.now += 10; node.loop(); }
  }
  uart_puts(radio.txCount > 0 ? "TX OK — mesh stack ran on ARM\r\n" : "no TX\r\n");
  for (;;) {}
}
