// The native node: real MeshCore, compiled for the host, with its radio on a
// socket to the simulator's RF engine.
//
// It is the same MeshCore sources and the same shims the emulated build uses.
// The only difference is what `Radio` is wired to — which is the point of
// ADR-0010: if native and emulated disagree, the disagreement is about the
// target, not about two different programs.
//
// This binary is distributed from a separate repository under MeshCore's own
// MIT licence, one release per architecture, because it links MeshCore.
#include <arpa/inet.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <sys/socket.h>
#include <unistd.h>

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <deque>
#include <string>
#include <vector>

#include "SimNode.h"

namespace {

constexpr uint8_t kFrame = 0x01;
constexpr uint8_t kTick = 0x02;
constexpr uint8_t kAck = 0x03;
constexpr uint8_t kTxDone = 0x04;
constexpr uint8_t kOriginate = 0x05;

// LoRa parameters. Defaults are MeshCore's UK/EU settings; the simulator will
// override them per node once board profiles land (MSIM-18).
int gSF = 10;
float gBandwidthKHz = 250.0f;
int gCodingRate = 1;  // 1..4 for 4/5..4/8

// RadioLib's getTimeOnAir(), truncated to milliseconds — which is exactly what
// MeshCore's RadioLibWrapper::getEstAirtimeFor() returns on real hardware.
//
// This has to be the firmware's own formula, not a good approximation of it.
// The CSMA backoff, the duty-cycle budget and the send timeout are all built on
// this number, so if the channel occupies the air for a different length of
// time than the firmware believes it did, the two desynchronise silently and
// every collision result after that is fiction. internal/dsp/airtime.go is the
// same calculation, tested against the published formula.
uint32_t loraAirtimeMs(int len) {
  float symbolMs = (float)((uint32_t)1 << gSF) / gBandwidthKHz;
  float sfCoeff1 = 4.25f, sfCoeff2 = 8.0f;
  if (gSF == 5 || gSF == 6) {
    sfCoeff1 = 6.25f;
    sfCoeff2 = 0.0f;
  }
  // The chip turns on low data rate optimisation itself once a symbol reaches
  // 16 ms — SF11 and SF12 at 125 kHz.
  int sfDivisor = (symbolMs >= 16.0f) ? 4 * (gSF - 2) : 4 * gSF;
  int bits = 8 * len + 16 /* CRC */ - 4 * gSF + (int)sfCoeff2 + 20 /* explicit header */;
  if (bits < 0) bits = 0;
  int coded = (bits + sfDivisor - 1) / sfDivisor;
  // MeshCore's own preamble rule, from RadioLibWrappers.h.
  int preamble = gSF <= 8 ? 32 : 16;
  float symbols = (float)preamble + sfCoeff1 + 8.0f + (float)(coded * (gCodingRate + 4));
  return (uint32_t)(symbolMs * symbols);
}

int connectTo(const std::string& addr) {
  auto colon = addr.rfind(':');
  if (colon == std::string::npos) {
    fprintf(stderr, "native: --bridge wants host:port, got %s\n", addr.c_str());
    return -1;
  }
  std::string host = addr.substr(0, colon);
  int port = atoi(addr.c_str() + colon + 1);

  int fd = socket(AF_INET, SOCK_STREAM, 0);
  if (fd < 0) return -1;
  sockaddr_in sa{};
  sa.sin_family = AF_INET;
  sa.sin_port = htons((uint16_t)port);
  if (inet_pton(AF_INET, host.c_str(), &sa.sin_addr) != 1) {
    close(fd);
    fprintf(stderr, "native: cannot parse address %s\n", host.c_str());
    return -1;
  }
  if (connect(fd, (sockaddr*)&sa, sizeof sa) != 0) {
    close(fd);
    return -1;
  }
  // Nagle would coalesce a frame with the tick that follows it, which is
  // exactly the latency the lockstep round trip is trying not to pay.
  int one = 1;
  setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, &one, sizeof one);
  return fd;
}

bool readAll(int fd, uint8_t* p, size_t n) {
  while (n) {
    ssize_t r = read(fd, p, n);
    if (r <= 0) return false;
    p += r;
    n -= (size_t)r;
  }
  return true;
}

bool writeMsg(int fd, uint8_t kind, const uint8_t* p, size_t n) {
  uint8_t hdr[3] = {kind, (uint8_t)(n >> 8), (uint8_t)n};
  if (write(fd, hdr, 3) != 3) return false;
  while (n) {
    ssize_t w = write(fd, p, n);
    if (w <= 0) return false;
    p += w;
    n -= (size_t)w;
  }
  return true;
}

// The one seam that differs between builds.
//
// Transmission goes out on the wire immediately and is *not* immediately
// complete: `isSendComplete()` stays false until the engine says the waveform
// has ended. The node does not time its own transmission — it cannot. How long
// the signal occupied the channel is a property of the samples the engine
// generated, and computing it here from the airtime estimate would replace the
// simulation with the formula.
//
// getEstAirtimeFor() below is a genuine estimate, which is all the firmware
// gets on real hardware too: it sizes CSMA backoff and spends the duty-cycle
// budget with it, before the packet is sent.
class BridgeRadio : public mesh::Radio {
 public:
  BridgeRadio(int fd, msim::SimClock& clk) : fd_(fd), clk_(clk) {}

  std::deque<std::vector<uint8_t>> inbox;

  bool startSendRaw(const uint8_t* bytes, int len) override {
    if (!writeMsg(fd_, kFrame, bytes, (size_t)len)) return false;
    sending_ = true;
    onAir_ = true;
    return true;
  }
  bool isSendComplete() override { return !onAir_; }
  void onSendFinished() override { sending_ = false; }

  // Called when the engine reports the waveform has left the antenna.
  void transmitFinished() { onAir_ = false; }

  int recvRaw(uint8_t* dest, int max) override {
    if (inbox.empty()) return 0;
    auto& f = inbox.front();
    int n = (int)f.size() < max ? (int)f.size() : max;
    memcpy(dest, f.data(), (size_t)n);
    inbox.pop_front();
    return n;
  }

  uint32_t getEstAirtimeFor(int len) override { return loraAirtimeMs(len); }
  // Ranking for the delayed-flood decision.
  //
  // MeshCore's own, from RadioLibWrapper::packetScoreInt, and it has to be:
  // Dispatcher::calcRxDelay computes (10^(0.85 - score) - 1) * airtime, which
  // assumes a score in [0,1]. The MSIM-1 probe carried a placeholder returning
  // snr*100 - len, and a score of 675 makes that expression negative, so every
  // node relayed the instant it decoded. Three repeaters transmitting at the
  // same millisecond is not a mesh — staggering by how well each heard the
  // packet is the whole of MeshCore's flood design, and a stub quietly removed
  // it while the simulation still looked plausible.
  float packetScore(float snr, int len) override {
    if (gSF < 7 || gSF > 12) return 0.0f;
    // Semtech's per-SF demodulator floor, the same table MeshCore uses.
    static const float threshold[] = {-7.5f, -10.0f, -12.5f, -15.0f, -17.5f, -20.0f};
    const float floorDB = threshold[gSF - 7];
    if (snr < floorDB) return 0.0f;  // no chance of success

    const float bySNR = (snr - floorDB) / 10.0f;
    const float collisionPenalty = 1.0f - (float)len / 256.0f;
    float v = bySNR * collisionPenalty;
    if (v < 0.0f) v = 0.0f;
    if (v > 1.0f) v = 1.0f;
    return v;
  }
  int getNoiseFloor() const override { return -117; }
  // Listening whenever it is not mid-transmission. The RF engine owns the real
  // answer; this is what the firmware can observe of it.
  bool isInRecvMode() const override { return !sending_; }

 private:
  int fd_;
  msim::SimClock& clk_;
  bool sending_ = false;
  bool onAir_ = false;
};

// Mesh's constructor is protected, so a node is a subclass — `using` would
// inherit it as protected and leave it unusable from main().
class NativeNode : public mesh::Mesh {
 public:
  NativeNode(mesh::Radio& r, mesh::MillisecondClock& c, mesh::RNG& g, mesh::RTCClock& t,
             mesh::PacketManager& m, mesh::MeshTables& tb)
      : mesh::Mesh(r, c, g, t, m, tb) {}
  int rxLogged = 0, txLogged = 0;
  void logRx(mesh::Packet*, int, float) override { rxLogged++; }
  void logTx(mesh::Packet*, int) override { txLogged++; }

  // Forwarding policy.
  //
  // This is the one place the node is not running MeshCore's own code, and it
  // is worth being precise about why. We link MeshCore's *library* — Mesh,
  // Dispatcher, Packet, Identity — which is where routing, retransmit timing,
  // duty-cycle accounting and CSMA live. We do not link the repeater
  // *application*, whose MyMesh::allowPacketForward also enforces region
  // transport codes and configurable loop-detect tables that need Arduino
  // preferences, an RTC and a filesystem.
  //
  // The rule below is that method's essential half: flood packets forward
  // until the hop cap. Without any override the base class refuses everything
  // and a flood stops dead at the origin's neighbours — which looks exactly
  // like a network with no repeaters configured. Recorded in
  // docs/shortcomings.md rather than left to be discovered.
  bool allowPacketForward(const mesh::Packet* packet) override {
    if (!packet->isRouteFlood()) return false;
    return packet->getPathHashCount() < floodMax;
  }

  // floodMax matches the repeater default. Beyond it a flood is spending
  // airtime to reach nodes that already have the message.
  static const int floodMax = 64;
};

}  // namespace

int main(int argc, char** argv) {
  std::string bridge;
  uint64_t seed = 4417;
  int printAirtimeFor = -1;
  for (int i = 1; i < argc - 1; i++) {
    if (!strcmp(argv[i], "--bridge")) bridge = argv[++i];
    else if (!strcmp(argv[i], "--seed")) seed = strtoull(argv[++i], nullptr, 10);
    else if (!strcmp(argv[i], "--sf")) gSF = atoi(argv[++i]);
    else if (!strcmp(argv[i], "--bw-khz")) gBandwidthKHz = (float)atof(argv[++i]);
    else if (!strcmp(argv[i], "--cr")) gCodingRate = atoi(argv[++i]);
    else if (!strcmp(argv[i], "--print-airtime")) printAirtimeFor = atoi(argv[++i]);
  }
  // A self-report, so the Go side can check that this transcription of the
  // airtime formula still agrees with internal/dsp. Two copies of a formula
  // that nothing compares are two formulas.
  if (printAirtimeFor >= 0) {
    printf("%u\n", loraAirtimeMs(printAirtimeFor));
    return 0;
  }
  if (bridge.empty()) {
    fprintf(stderr, "usage: %s --bridge host:port [--seed N] [--sf N] [--bw-khz F] [--cr N]\n", argv[0]);
    return 2;
  }
  int fd = connectTo(bridge);
  if (fd < 0) {
    fprintf(stderr, "native: cannot reach the simulator at %s\n", bridge.c_str());
    return 1;
  }

  msim::SimClock clk;
  msim::SimRNG rng(seed);
  msim::SimRTC rtc;
  msim::SimTables tables;
  msim::SimPacketMgr mgr;
  BridgeRadio radio(fd, clk);
  NativeNode node(radio, clk, rng, rtc, mgr, tables);
  node.begin();
  fprintf(stderr, "native: MeshCore up, seed=%llu SF%d BW%.0fkHz CR4/%d\n",
          (unsigned long long)seed, gSF, gBandwidthKHz, gCodingRate + 4);

  uint8_t hdr[3];
  for (;;) {
    if (!readAll(fd, hdr, 3)) break;
    uint16_t n = (uint16_t)((hdr[1] << 8) | hdr[2]);
    std::vector<uint8_t> payload(n);
    if (n && !readAll(fd, payload.data(), n)) break;

    switch (hdr[0]) {
      case kFrame:
        // Queued, not delivered: the firmware collects it from recvRaw() on its
        // next loop, exactly as it would drain a real radio's FIFO.
        radio.inbox.push_back(std::move(payload));
        break;

      case kTxDone:
        radio.transmitFinished();
        break;

      case kOriginate: {
        // Built by the firmware, so it is a packet the rest of the firmware
        // will accept. A frame fabricated on the host is not, and every node
        // that receives one drops it — correctly, and silently.
        mesh::Packet* p = mgr.allocNew();
        if (!p) {
          fprintf(stderr, "native: packet pool empty, cannot originate\n");
          break;
        }
        int n = (int)payload.size();
        if (n > MAX_PACKET_PAYLOAD) n = MAX_PACKET_PAYLOAD;
        // A group text message: the flood-routed type a mesh actually carries,
        // so relay decisions here are the ones MeshCore makes for real traffic
        // rather than for something invented.
        p->header = (PAYLOAD_TYPE_GRP_TXT << PH_TYPE_SHIFT) | ROUTE_TYPE_FLOOD;
        p->payload_len = (uint8_t)n;
        for (int i = 0; i < n; i++) p->payload[i] = payload[(size_t)i];
        node.sendFlood(p);
        break;
      }

      case kTick: {
        if (n != 4) break;
        uint32_t at = ((uint32_t)payload[0] << 24) | ((uint32_t)payload[1] << 16) |
                      ((uint32_t)payload[2] << 8) | payload[3];
        // One loop per millisecond of simulated time. Stepping rather than
        // jumping is what keeps timeouts, retries and duty-cycle refill
        // behaving as they do on hardware; a node that sees time move in
        // 500 ms jumps takes different branches.
        while (clk.now < at) {
          clk.now++;
          node.loop();
        }
        clk.now = at;
        node.loop();
        uint8_t ack[4] = {payload[0], payload[1], payload[2], payload[3]};
        if (!writeMsg(fd, kAck, ack, 4)) goto done;
        break;
      }

      default:
        goto done;
    }
  }
done:
  fprintf(stderr, "native: bridge closed after %lu ms, tx=%d rx=%d\n", clk.now, node.txLogged,
          node.rxLogged);
  close(fd);
  return 0;
}
