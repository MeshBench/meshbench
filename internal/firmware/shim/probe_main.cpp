// MSIM-1 proof: the real MeshCore mesh stack, compiled for the host and driven
// through our own hardware shims. Nothing in MeshCore is modified — every seam
// used here is one upstream already declares.
#include <Mesh.h>
#include <Dispatcher.h>
#include <Utils.h>
#include <cstdio>
#include <vector>

using namespace mesh;

// ── the seam the whole design rests on ───────────────────────────────────
class SimRadio : public Radio {
public:
  int txCount = 0;
  bool startSendRaw(const uint8_t* bytes, int len) override {
    txCount++;
    printf("  [SimRadio] startSendRaw len=%d bytes:", len);
    for (int i = 0; i < (len < 8 ? len : 8); i++) printf(" %02x", bytes[i]);
    printf("\n");
    return true;
  }
  bool isSendComplete() override { return true; }
  void onSendFinished() override {}
  int recvRaw(uint8_t*, int) override { return 0; }
  uint32_t getEstAirtimeFor(int len) override { return (uint32_t)(len * 8 * 1000 / 1758); }
  float packetScore(float snr, int len) override { return snr * 100 - len; }
  int getNoiseFloor() const override { return -117; }
  // True whenever the radio is listening — in the real simulator this comes
  // from the RF engine's channel state, which is what makes CAD truthful.
  bool isInRecvMode() const override { return true; }
};

class SimClock : public MillisecondClock {
public:
  unsigned long now = 0;
  unsigned long getMillis() override { return now; }
};
class SimRTC : public RTCClock {
public:
  uint32_t t = 1754700000;
  uint32_t getCurrentTime() override { return t; }
  void setCurrentTime(uint32_t v) override { t = v; }
};
class SimRNG : public RNG {
public:
  uint64_t state = 4417;   // seeded — determinism is a requirement
  void random(uint8_t* dest, size_t sz) override {
    for (size_t i = 0; i < sz; i++) {
      state = state * 6364136223846793005ULL + 1442695040888963407ULL;
      dest[i] = (uint8_t)(state >> 33);
    }
  }
};
class SimTables : public MeshTables {
public:
  std::vector<uint32_t> seen;
  bool wasSeen(const Packet* p) override {
    uint32_t h = hash(p);
    for (auto v : seen) if (v == h) return true;
    return false;
  }
  void markSeen(const Packet* p) override { seen.push_back(hash(p)); }
  void clear(const Packet* p) override {
    uint32_t h = hash(p);
    for (size_t i = 0; i < seen.size(); i++)
      if (seen[i] == h) { seen.erase(seen.begin() + i); return; }
  }
private:
  static uint32_t hash(const Packet* p) {
    uint32_t h = 2166136261u;
    for (int i = 0; i < p->payload_len; i++) { h ^= p->payload[i]; h *= 16777619u; }
    return h ^ (uint32_t)p->header;
  }
};
class SimPacketMgr : public PacketManager {
public:
  SimPacketMgr() { for (int i = 0; i < 32; i++) pool.push_back(new Packet()); }
  Packet* allocNew() override {
    if (pool.empty()) return nullptr;
    Packet* p = pool.back(); pool.pop_back(); return p;
  }
  void free(Packet* p) override { if (p) pool.push_back(p); }
  void queueOutbound(Packet* p, uint8_t, uint32_t when) override { out.push_back({p, when}); }
  Packet* getNextOutbound(uint32_t now) override {
    for (size_t i = 0; i < out.size(); i++)
      if (out[i].when <= now) { Packet* p = out[i].p; out.erase(out.begin() + i); return p; }
    return nullptr;
  }
  int getOutboundCount(uint32_t now) const override {
    int n = 0; for (auto& e : out) if (e.when <= now) n++; return n;
  }
  int getOutboundTotal() const override { return (int)out.size(); }
  int getFreeCount() const override { return (int)pool.size(); }
  Packet* getOutboundByIdx(int i) override { return (i >= 0 && i < (int)out.size()) ? out[i].p : nullptr; }
  Packet* removeOutboundByIdx(int i) override {
    if (i < 0 || i >= (int)out.size()) return nullptr;
    Packet* p = out[i].p; out.erase(out.begin() + i); return p;
  }
  void queueInbound(Packet* p, uint32_t when) override { in.push_back({p, when}); }
  Packet* getNextInbound(uint32_t now) override {
    for (size_t i = 0; i < in.size(); i++)
      if (in[i].when <= now) { Packet* p = in[i].p; in.erase(in.begin() + i); return p; }
    return nullptr;
  }
private:
  struct E { Packet* p; uint32_t when; };
  std::vector<Packet*> pool;
  std::vector<E> out, in;
};

// Observation layer 2: upstream's own empty virtuals, overridden. Not a fork.
class SimNode : public Mesh {
public:
  int rxLogged = 0, txLogged = 0;
  SimNode(SimRadio& r, SimClock& c, SimRNG& g, SimRTC& t, SimPacketMgr& m, SimTables& tb)
      : Mesh(r, c, g, t, m, tb) {}
  void logRx(Packet*, int, float) override { rxLogged++; }
  void logTx(Packet*, int) override { txLogged++; }
};

int main() {
  SimRadio radio; SimClock clk; SimRNG rng; SimRTC rtc; SimPacketMgr mgr; SimTables tables;
  SimNode node(radio, clk, rng, rtc, mgr, tables);

  printf("MSIM-1: real MeshCore mesh stack, host build\n");
  printf("  packet pool free: %d\n", mgr.getFreeCount());

  LocalIdentity id(&rng);
  printf("  identity generated, pub_key[0..3]: %02x %02x %02x %02x\n",
         id.pub_key[0], id.pub_key[1], id.pub_key[2], id.pub_key[3]);

  node.begin();
  printf("  Mesh::begin() ok\n");

  for (int i = 0; i < 50; i++) { clk.now += 10; node.loop(); }
  printf("  50 loops @10ms -> txCount=%d logTx=%d logRx=%d\n",
         radio.txCount, node.txLogged, node.rxLogged);

  // Prove the Radio seam is genuinely wired: hand the firmware a packet to send.
  Packet* p = mgr.allocNew();
  if (p) {
    p->header = 0x12;
    p->payload_len = 8;
    for (int i = 0; i < 8; i++) p->payload[i] = (uint8_t)(0xA0 + i);
    node.sendFlood(p);
    for (int i = 0; i < 200; i++) { clk.now += 10; node.loop(); }
  }
  printf("  after sendFlood -> txCount=%d logTx=%d\n", radio.txCount, node.txLogged);
  printf("RESULT: %s\n", radio.txCount > 0
         ? "firmware transmitted through SimRadio — seam confirmed"
         : "no transmission observed");
  return 0;
}
