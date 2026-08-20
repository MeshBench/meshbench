// The host-side hardware shims MeshCore needs, shared by the probe and the
// native node.
//
// Everything here implements an abstract class upstream already declares —
// nothing in MeshCore is modified. `Radio` is deliberately absent: it is the
// one seam whose implementation differs between builds (the probe prints, the
// native node puts frames on a socket), and pretending otherwise would mean a
// base class with one real user.
#pragma once

#include <Dispatcher.h>
#include <Mesh.h>
#include <Utils.h>

#include <cstdint>
#include <vector>

namespace msim {

// Supplied by the simulator, never read from the host. A node that consults
// wall-clock time cannot be replayed, and two nodes that do it independently
// stop agreeing about what happened first.
class SimClock : public mesh::MillisecondClock {
 public:
  unsigned long now = 0;
  unsigned long getMillis() override { return now; }
};

// Wall-clock seconds, derived from the simulated millisecond clock beside it
// rather than kept independently.
//
// It used to hold a constant that only setCurrentTime moved, so every node
// believed the whole run happened in one instant. MeshCore stamps each advert
// with getCurrentTime() and dedups received packets by hash, so a node told to
// advert twice emitted byte-identical packets and every repeater dropped the
// second - correctly. A node could advert once per run and never again.
//
// Derived, not counted: the same seed still produces the same run, because the
// only thing this reads is simulated time. setCurrentTime keeps working, as an
// offset, so `clock sync` still moves a node's idea of the date without
// detaching it from the clock everything else uses.
class SimRTC : public mesh::RTCClock {
 public:
  explicit SimRTC(const SimClock& clk, uint32_t start = 1754700000)
      : clk(clk), base(start) {}
  uint32_t getCurrentTime() override {
    return base + (uint32_t)(clk.now / 1000);
  }
  void setCurrentTime(uint32_t v) override {
    // Rebase rather than freeze: whatever the node is told the time is, it
    // goes on advancing from there at the rate the simulation does.
    base = v - (uint32_t)(clk.now / 1000);
  }

 private:
  const SimClock& clk;
  uint32_t base;
};

// Seeded rather than entropic: identity generation must be reproducible, or the
// same scenario produces different public keys every run and no two runs can be
// compared.
class SimRNG : public mesh::RNG {
 public:
  explicit SimRNG(uint64_t seed = 4417) : state(seed ? seed : 4417) {}
  void random(uint8_t* dest, size_t sz) override {
    for (size_t i = 0; i < sz; i++) {
      state = state * 6364136223846793005ULL + 1442695040888963407ULL;
      dest[i] = (uint8_t)(state >> 33);
    }
  }

 private:
  uint64_t state;
};

class SimTables : public mesh::MeshTables {
 public:
  bool wasSeen(const mesh::Packet* p) override {
    uint32_t h = hash(p);
    for (auto v : seen)
      if (v == h) return true;
    return false;
  }
  void markSeen(const mesh::Packet* p) override { seen.push_back(hash(p)); }
  void clear(const mesh::Packet* p) override {
    uint32_t h = hash(p);
    for (size_t i = 0; i < seen.size(); i++)
      if (seen[i] == h) {
        seen.erase(seen.begin() + i);
        return;
      }
  }

 private:
  static uint32_t hash(const mesh::Packet* p) {
    uint32_t h = 2166136261u;
    for (int i = 0; i < p->payload_len; i++) {
      h ^= p->payload[i];
      h *= 16777619u;
    }
    return h ^ (uint32_t)p->header;
  }
  std::vector<uint32_t> seen;
};

class SimPacketMgr : public mesh::PacketManager {
 public:
  SimPacketMgr() {
    for (int i = 0; i < 32; i++) pool.push_back(new mesh::Packet());
  }
  mesh::Packet* allocNew() override {
    if (pool.empty()) return nullptr;
    mesh::Packet* p = pool.back();
    pool.pop_back();
    return p;
  }
  void free(mesh::Packet* p) override {
    if (p) pool.push_back(p);
  }
  void queueOutbound(mesh::Packet* p, uint8_t, uint32_t when) override { out.push_back({p, when}); }
  mesh::Packet* getNextOutbound(uint32_t now) override { return take(out, now); }
  int getOutboundCount(uint32_t now) const override {
    int n = 0;
    for (auto& e : out)
      if (e.when <= now) n++;
    return n;
  }
  int getOutboundTotal() const override { return (int)out.size(); }
  int getFreeCount() const override { return (int)pool.size(); }
  mesh::Packet* getOutboundByIdx(int i) override {
    return (i >= 0 && i < (int)out.size()) ? out[i].p : nullptr;
  }
  mesh::Packet* removeOutboundByIdx(int i) override {
    if (i < 0 || i >= (int)out.size()) return nullptr;
    mesh::Packet* p = out[i].p;
    out.erase(out.begin() + i);
    return p;
  }
  void queueInbound(mesh::Packet* p, uint32_t when) override { in.push_back({p, when}); }
  mesh::Packet* getNextInbound(uint32_t now) override { return take(in, now); }

 private:
  struct E {
    mesh::Packet* p;
    uint32_t when;
  };
  static mesh::Packet* take(std::vector<E>& q, uint32_t now) {
    for (size_t i = 0; i < q.size(); i++)
      if (q[i].when <= now) {
        mesh::Packet* p = q[i].p;
        q.erase(q.begin() + i);
        return p;
      }
    return nullptr;
  }
  std::vector<mesh::Packet*> pool;
  std::vector<E> out, in;
};

}  // namespace msim
