// MSIM-1 proof: the real MeshCore mesh stack, compiled for the host and driven
// through our own hardware shims. Nothing in MeshCore is modified — every seam
// used here is one upstream already declares.
//
// The shims themselves live in SimNode.h, shared with the native node: a proof
// that exercises a different stack from the one that ships proves nothing.
#include <cstdio>

#include "SimNode.h"

using namespace mesh;
using msim::SimClock;
using msim::SimPacketMgr;
using msim::SimRNG;
using msim::SimRTC;
using msim::SimTables;

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
