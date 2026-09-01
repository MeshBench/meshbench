// A host filesystem with the Adafruit_LittleFS surface MeshCore's nRF52 build
// expects.
//
// The repeater keeps its identity, its preferences and its client ACL in flash,
// and the CLI writes to them. Stubbing that out would give a node that forgets
// who it is on every restart and a CLI whose settings do nothing — so the files
// are real files, under a directory the simulator owns.
//
// nRF52 rather than the ESP32 branch because the board we also emulate is an
// nRF52840, and having both backends take the same path through MeshCore is
// the point of ADR-0010's cross-check.
#pragma once

#include <dirent.h>
#include <stdint.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

#include <string>

#include <Stream.h>

#define FILE_O_READ "rb"
#define FILE_O_WRITE "wb"

namespace Adafruit_LittleFS_Namespace {

// Derived from Stream, as Arduino's File is. CommonCLI hands one to anything
// taking a Stream&, and Identity::readFrom/writeTo are declared on Stream — so
// this is not a convenience, it is the type relationship MeshCore is written
// against.
class File : public Stream {
 public:
  File() = default;
  File(FILE* f, const std::string& path) : f_(f), path_(path) {}

  explicit operator bool() const { return f_ != nullptr; }
  bool operator!() const { return f_ == nullptr; }

  int read() override {
    if (!f_) return -1;
    int c = fgetc(f_);
    return c == EOF ? -1 : c;
  }
  int peek() override { return -1; }
  void flush() override {
    if (f_) fflush(f_);
  }
  int available() override { return f_ ? (int)(size() - (uint32_t)ftell(f_)) : 0; }

  size_t readBytes(uint8_t* buf, size_t len) {
    if (!f_) return 0;
    return fread(buf, 1, len, f_);
  }
  int read(uint8_t* buf, size_t len) { return (int)readBytes(buf, len); }

  size_t write(const uint8_t* buf, size_t len) override {
    if (!f_) return 0;
    return fwrite(buf, 1, len, f_);
  }
  size_t write(uint8_t b) override { return write(&b, 1); }

  // The repeater writes its preferences and its ACL as text, so print and
  // printf are not conveniences here — they are how those files are produced.
  size_t print(const char* s) { return s ? write((const uint8_t*)s, strlen(s)) : 0; }
  size_t print(int v) { return printf("%d", v); }
  size_t println(const char* s) { return print(s) + print("\n"); }
  size_t printf(const char* fmt, ...) {
    if (!f_) return 0;
    va_list ap;
    va_start(ap, fmt);
    int n = vfprintf(f_, fmt, ap);
    va_end(ap);
    return n < 0 ? 0 : (size_t)n;
  }

  bool seek(uint32_t pos) { return f_ && fseek(f_, (long)pos, SEEK_SET) == 0; }
  uint32_t size() {
    if (!f_) return 0;
    long here = ftell(f_);
    fseek(f_, 0, SEEK_END);
    long end = ftell(f_);
    fseek(f_, here, SEEK_SET);
    return (uint32_t)end;
  }
  void close() {
    if (f_) {
      fclose(f_);
      f_ = nullptr;
    }
  }
  const char* name() const { return path_.c_str(); }

 private:
  FILE* f_ = nullptr;
  std::string path_;
};

}  // namespace Adafruit_LittleFS_Namespace

using Adafruit_LittleFS_Namespace::File;

class Adafruit_LittleFS {
 public:
  // Root is set by the simulator so each node gets its own storage. Sharing one
  // directory between nodes would give every repeater the same identity, which
  // is not a mesh.
  void setRoot(const std::string& dir) { root_ = dir; ensureDir(root_); }

  bool begin() { ensureDir(root_); return true; }

  bool exists(const char* path) {
    struct stat st {};
    return stat(full(path).c_str(), &st) == 0;
  }
  bool mkdir(const char* path) { return ensureDir(full(path)); }
  bool remove(const char* path) { return ::remove(full(path).c_str()) == 0; }

  File open(const char* path, const char* mode = FILE_O_READ) {
    std::string p = full(path);
    // A write to a path whose directory does not exist is the commonest way
    // this fails, and it fails silently as a null file.
    auto slash = p.find_last_of('/');
    if (slash != std::string::npos) ensureDir(p.substr(0, slash));
    FILE* f = fopen(p.c_str(), mode);
    return File(f, p);
  }

  // Erase the node's storage, as the CLI's own format command means it.
  //
  // Bounded to the directory the simulator handed this node, and never to a
  // caller-supplied path: a CLI bug that reached a recursive delete of an
  // arbitrary argument would take somebody's home directory with it. Within
  // that bound it has to really erase, because a node that answers "done" and
  // keeps its identity is a node the operator believes they have reset, and
  // every result taken after that is about a node that is not the one they
  // think they are looking at.
  bool format() {
    if (root_.empty()) return false;
    if (!clearDir(root_)) return false;
    return ensureDir(root_);
  }

 private:
  static bool clearDir(const std::string& dir) {
    DIR* d = opendir(dir.c_str());
    if (!d) return false;
    bool ok = true;
    while (struct dirent* e = readdir(d)) {
      std::string name = e->d_name;
      if (name == "." || name == "..") continue;
      std::string p = dir + "/" + name;
      // lstat, not stat: a symlink is removed as a link, and following one
      // would let a link inside the node's directory reach outside it.
      struct stat st {};
      if (::lstat(p.c_str(), &st) != 0) {
        ok = false;
        continue;
      }
      bool gone = S_ISDIR(st.st_mode) ? clearDir(p) && ::rmdir(p.c_str()) == 0
                                      : ::remove(p.c_str()) == 0;
      ok = ok && gone;
    }
    closedir(d);
    return ok;
  }

  std::string full(const char* path) const {
    std::string p = path ? path : "";
    if (!p.empty() && p[0] == '/') p.erase(0, 1);
    return root_.empty() ? p : root_ + "/" + p;
  }
  static bool ensureDir(const std::string& dir) {
    if (dir.empty()) return true;
    std::string acc;
    for (size_t i = 0; i <= dir.size(); i++) {
      if (i == dir.size() || dir[i] == '/') {
        if (!acc.empty()) ::mkdir(acc.c_str(), 0755);
      }
      if (i < dir.size()) acc += dir[i];
    }
    return true;
  }
  std::string root_;
};

extern Adafruit_LittleFS InternalFS;
