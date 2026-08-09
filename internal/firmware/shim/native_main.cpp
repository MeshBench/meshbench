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

// The one seam that differs between builds. Transmission is immediate on the
// wire and *not* immediately complete: `isSendComplete()` stays false for the
// packet's own airtime, so the firmware's duty-cycle accounting and CSMA see
// the same timing they would on hardware. Reporting completion straight away
// is the single easiest way to make a simulator optimistic.
class BridgeRadio : public mesh::Radio {
 public:
  BridgeRadio(int fd, msim::SimClock& clk) : fd_(fd), clk_(clk) {}

  std::deque<std::vector<uint8_t>> inbox;

  bool startSendRaw(const uint8_t* bytes, int len) override {
    if (!writeMsg(fd_, kFrame, bytes, (size_t)len)) return false;
    sendDone_ = clk_.now + getEstAirtimeFor(len);
    sending_ = true;
    return true;
  }
  bool isSendComplete() override { return !sending_ || clk_.now >= sendDone_; }
  void onSendFinished() override { sending_ = false; }

  int recvRaw(uint8_t* dest, int max) override {
    if (inbox.empty()) return 0;
    auto& f = inbox.front();
    int n = (int)f.size() < max ? (int)f.size() : max;
    memcpy(dest, f.data(), (size_t)n);
    inbox.pop_front();
    return n;
  }

  uint32_t getEstAirtimeFor(int len) override { return loraAirtimeMs(len); }
  // Ranking for the delayed-flood decision. Kept identical to the emulated
  // build so a route chosen natively is the route chosen under emulation.
  float packetScore(float snr, int len) override { return snr * 100 - len; }
  int getNoiseFloor() const override { return -117; }
  // Listening whenever it is not mid-transmission. The RF engine owns the real
  // answer; this is what the firmware can observe of it.
  bool isInRecvMode() const override { return !sending_; }

 private:
  int fd_;
  msim::SimClock& clk_;
  bool sending_ = false;
  unsigned long sendDone_ = 0;
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
