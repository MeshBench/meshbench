// Package packet decodes the MeshCore frame: what the bytes on the air mean.
//
// Split out of internal/capture, which was doing two jobs. Dissecting a frame
// is protocol knowledge and belongs beside the firmware that speaks it;
// writing a pcapng file and counting who heard what is a record of a
// simulation. The two halves shared not one symbol, which is usually the sign
// that a package is two.
//
// internal/provider is why it mattered: a live feed from CoreScope or Beacon
// needs to read frames off the air, and had to import the simulation's
// recording layer to do it.
package packet
