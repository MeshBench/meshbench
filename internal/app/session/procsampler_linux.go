//go:build linux

package session

// procSamplerWorks says whether this platform has the /proc the sampler reads.
//
// Linux only, and said rather than assumed. The sampler read /proc
// unconditionally and returned zeroes when it was not there, so on macOS and
// Windows every node reported 0 CPU and 0 RSS - numbers a caller believes,
// indistinguishable from a node that genuinely costs nothing.
const procSamplerWorks = true
