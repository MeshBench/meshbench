package regression

// PathologicalCases is plan §10's own suite: six shipped network shapes -
// spider, bridge, loop, trap, long path, boundary - each with one named
// assertion about the property that shape exists to exercise (fan-out and
// dedupe, a single point of failure, redundant paths, a dead end that must
// not storm, a chain many hops deep, and a region that must not leak). Built
// here rather than read from a directory: the fixtures are embedded in the
// binary already (fixtures/fixture-pathological-*.json), and a suite that
// only worked when installed next to a *.json case directory would not
// survive being packaged the way the rest of the application is - see
// fixtures/embed.go's own note on why the shipped scenarios are embedded at
// all. baseDir is always "" for these: Fixture names an embedded fixture,
// resolved by fixture.Find, never a path on disk.
func PathologicalCases() []Case {
	shape := func(name string, seed uint64) Case {
		return Case{
			FormatVersion: caseFormatVersion,
			Name:          name,
			Fixture:       "fixture-" + name,
			Seeds:         []uint64{seed},
			ForMs:         150_000,
		}
	}
	return []Case{
		shape("pathological-spider", 4001),
		shape("pathological-bridge", 4002),
		shape("pathological-loop", 4003),
		shape("pathological-trap", 4004),
		shape("pathological-longpath", 4005),
		shape("pathological-boundary", 4006),
	}
}
