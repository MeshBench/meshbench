package coverage

import "image/color"

// Ramp is the colour a link margin is drawn in: the identity's waterfall,
// read from the noise floor up.
//
// A violet floor rising through the mid band to a hot orange at the top, which
// is the same colormap a spectrogram is read with and the reason the identity
// has orange mean signal at all. Strong margin is therefore hot and weak is
// cold, which is the way round somebody who has looked at a waterfall already
// expects, and the way round the brand requires: orange marks traffic, never a
// verdict.
//
// It was the other way round - orange at the floor, green at 20 dB - which read
// as a warning colour for thin coverage and put the identity's one reserved
// colour on a status. HopReach's decision to keep it continuous is kept: bands
// made a smooth physical quantity look like four verdicts, and the eye reads a
// gradient's shape where it only counts a band's edges.
//
// Zero decibels is the floor and 20 is the top, because more margin than that
// is not a distinction anybody acts on. Alpha is the caller's: the overlay has
// an opacity slider and the legend does not.
// rampStops are the brand's five. Interpolating between them rather than
// across the whole run keeps the mid band violet instead of letting it wash
// through grey, which is what a straight floor-to-top lerp would do.
var rampStops = [...]color.RGBA{
	{R: 0x14, G: 0x0a, B: 0x3e, A: 0xff},
	{R: 0x3a, G: 0x25, B: 0x97, A: 0xff},
	{R: 0x6b, G: 0x45, B: 0xd8, A: 0xff},
	{R: 0xe8, G: 0x50, B: 0x0f, A: 0xff},
	{R: 0xff, G: 0xb1, B: 0x3d, A: 0xff},
}

func Ramp(marginDB float64) color.RGBA {
	t := marginDB / 20
	switch {
	case t < 0:
		t = 0
	case t > 1:
		t = 1
	}
	stops := rampStops
	span := 1.0 / float64(len(stops)-1)
	for i := 0; i+1 < len(stops); i++ {
		if t > float64(i+1)*span && i+2 < len(stops) {
			continue
		}
		f := (t - float64(i)*span) / span
		lo, hi := stops[i], stops[i+1]
		lerp := func(a, b uint8) uint8 { return uint8(float64(a) + f*(float64(b)-float64(a))) }
		return color.RGBA{R: lerp(lo.R, hi.R), G: lerp(lo.G, hi.G), B: lerp(lo.B, hi.B), A: 0xff}
	}
	return stops[len(stops)-1]
}
