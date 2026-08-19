// Package propagation is the path-loss maths: heights in, decibels out.
//
// It knows about ground, frequency and geometry, and nothing about nodes,
// networks or what a result will be drawn as. That is the whole reason it is
// its own package: every function here has a twin in internal/gpu, and the
// pair is held together by an equivalence test. When the CPU side lived in
// internal/coverage - beside the raster, the combiner and the best-server
// search - the GPU package had to import all of that to reach three functions,
// which is what an import graph looks like just before it stops meaning
// anything.
//
// The split runs along the line CLAUDE.md already draws. A kernel and its CPU
// twin are physics; a raster of margins is an answer to a question. This is
// the first half.
package propagation
