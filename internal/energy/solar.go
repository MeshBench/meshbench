package energy

import "math"

// Panel is a photovoltaic panel as mounted, not as sold.
type Panel struct {
	// PeakW at 1000 W/m² and 25 °C — the number on the label.
	PeakW float64

	// TiltDeg from horizontal and AzimuthDeg as a compass bearing. A panel
	// bolted flat to the top of a pole is Tilt 0, and in Scotland that is a
	// materially worse panel than the same one at 50°.
	TiltDeg    float64
	AzimuthDeg float64

	// SoilingFactor in [0,1] for dirt, bird mess and winter grime. 0.9 is a
	// panel someone visits; 0.7 is one on a hill that nobody has touched.
	SoilingFactor float64

	// ChargeEfficiency of the controller. A PWM controller on a 12 V panel
	// charging a Li-ion pack throws away a lot; MPPT rather less. 0.75 PWM,
	// 0.95 MPPT.
	ChargeEfficiency float64
}

// Sun is where the sun is, and how much of it gets through.
type Sun struct {
	// ElevationDeg above the horizon. Negative means night.
	ElevationDeg float64
	// AzimuthDeg is its compass bearing.
	AzimuthDeg float64
	// DirectNormalWM2 is clear-sky direct irradiance on a surface facing it.
	DirectNormalWM2 float64
	// DiffuseWM2 is skylight, which is what a panel lives on under overcast —
	// and in a British winter that is most of the resource, so leaving it out
	// would make every panel look useless.
	DiffuseWM2 float64
}

// SunAt computes the sun's position and clear-sky irradiance.
//
// latDeg north-positive, dayOfYear 1..365, hourUTC in decimal hours, lonDeg
// east-positive. The solar-position maths is the standard NOAA approximation,
// which is good to a fraction of a degree — far better than the cloud model it
// feeds, so it is not the limiting error.
func SunAt(latDeg, lonDeg float64, dayOfYear int, hourUTC float64) Sun {
	rad := math.Pi / 180

	// Declination, and the equation of time in minutes.
	g := 2 * math.Pi / 365 * (float64(dayOfYear) - 1 + (hourUTC-12)/24)
	decl := 0.006918 - 0.399912*math.Cos(g) + 0.070257*math.Sin(g) -
		0.006758*math.Cos(2*g) + 0.000907*math.Sin(2*g) -
		0.002697*math.Cos(3*g) + 0.00148*math.Sin(3*g)
	eqTime := 229.18 * (0.000075 + 0.001868*math.Cos(g) - 0.032077*math.Sin(g) -
		0.014615*math.Cos(2*g) - 0.040849*math.Sin(2*g))

	trueSolarTime := hourUTC*60 + eqTime + 4*lonDeg
	hourAngle := (trueSolarTime/4 - 180) * rad

	lat := latDeg * rad
	cosZenith := math.Sin(lat)*math.Sin(decl) + math.Cos(lat)*math.Cos(decl)*math.Cos(hourAngle)
	cosZenith = clamp(cosZenith, -1, 1)
	elevation := math.Asin(cosZenith) / rad

	az := math.Atan2(-math.Sin(hourAngle),
		math.Tan(decl)*math.Cos(lat)-math.Sin(lat)*math.Cos(hourAngle)) / rad
	if az < 0 {
		az += 360
	}

	s := Sun{ElevationDeg: elevation, AzimuthDeg: az}
	if elevation <= 0 {
		return s
	}

	// Kasten-Young air mass, which stays finite at the horizon where the naive
	// 1/cos(z) does not — and low sun is exactly the case that decides whether
	// a northern site survives December.
	zenith := 90 - elevation
	am := 1 / (cosZenith + 0.50572*math.Pow(96.07995-zenith, -1.6364))
	s.DirectNormalWM2 = 1353 * math.Pow(0.7, math.Pow(am, 0.678))
	// Simple diffuse fraction. Roughly 10% of the extraterrestrial normal on a
	// clear day, rising as the sun drops.
	s.DiffuseWM2 = 0.1 * 1353 * cosZenith
	return s
}

// HarvestW is the electrical power a panel delivers under a given sun and mean
// cloud cover in [0,1], where 1 is total overcast.
//
// The cloud model is Kasten-Czeplak for global irradiance and an isotropic-sky
// transposition (Liu-Jordan) onto the tilted plane. Both are published
// relations rather than something plausible-looking, and that matters here: an
// earlier version suppressed the direct beam as (1-N)^3, which at a *mean*
// monthly cloud cover of 0.8 removes 99% of it. Mean cloud does not mean every
// hour is overcast, and a strongly non-linear function of a mean is not the
// mean of the function. The visible symptom was a flat panel beating a tilted
// one at 57 degrees north in December, which no installer would recognise.
func (p Panel) HarvestW(s Sun, cloudCover float64) float64 {
	if p.PeakW <= 0 || s.ElevationDeg <= 0 {
		return 0
	}
	cloud := clamp(cloudCover, 0, 1)
	rad := math.Pi / 180
	sinElev := math.Sin(s.ElevationDeg * rad)

	// Clear-sky global horizontal, then Kasten-Czeplak for cloud.
	clearGHI := s.DirectNormalWM2*sinElev + s.DiffuseWM2
	ghi := clearGHI * (1 - 0.75*math.Pow(cloud, 3.4))

	// Diffuse fraction rises with cloud: roughly a quarter of the resource on a
	// clear day, all of it under total overcast.
	diffuseFraction := clamp(0.25+0.75*cloud, 0, 1)
	diffuseHorizontal := ghi * diffuseFraction
	directHorizontal := ghi - diffuseHorizontal

	// Back out the direct normal component that survives.
	dni := 0.0
	if sinElev > 0.01 {
		dni = directHorizontal / sinElev
	}

	tilt := p.TiltDeg * rad
	incidence := sinElev*math.Cos(tilt) +
		math.Cos(s.ElevationDeg*rad)*math.Sin(tilt)*math.Cos((s.AzimuthDeg-p.AzimuthDeg)*rad)

	plane := 0.0
	if incidence > 0 {
		plane += dni * incidence
	}
	// Isotropic sky: a tilted panel sees less of it.
	plane += diffuseHorizontal * (1 + math.Cos(tilt)) / 2
	// Ground reflection, which is the term that stops a steep winter tilt from
	// being penalised as heavily as the sky-view loss alone would suggest.
	// Albedo 0.2 for grass and heather; snow would be three times that, and
	// snow on the ground is exactly when a steep panel earns its keep.
	const albedo = 0.2
	plane += ghi * albedo * (1 - math.Cos(tilt)) / 2

	soiling := p.SoilingFactor
	if soiling <= 0 {
		soiling = 0.9
	}
	eff := p.ChargeEfficiency
	if eff <= 0 {
		eff = 0.95
	}
	return p.PeakW * plane / 1000 * soiling * eff
}
