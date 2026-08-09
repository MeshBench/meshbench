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

class SimRadio : public Radio {
public:
  volatile int txCount = 0;
  bool startSendRaw(const uint8_t*, int) override { txCount++; return true; }
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
  uart_puts("MSIM bare-metal nRF52840, no SoftDevice\r\n");

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
