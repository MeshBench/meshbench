//go:build !linux

package session

// procSamplerWorks is false where there is no /proc to read. See the Linux
// file for why this is a constant somebody has to look at rather than a read
// that quietly fails.
const procSamplerWorks = false
