// Package regression runs a saved fixture against real firmware and decides
// whether it still passes - the machinery behind "will this stay fixed?".
//
// A fixture already carries a network and its assertions; what it does not
// carry is which seeds to run it at, which firmware to pin, or how far a
// stochastic metric is allowed to drift before that is a regression rather
// than noise. Case adds exactly those three things, as one portable file -
// "a bug report that runs itself" - and RunDir is the thing CI calls.
package regression

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/coverage"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/fixture"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// caseFormatVersion is stamped on every case this package writes. A case
// missing it, or newer than this build knows, is refused rather than run as
// though its format were understood - the same rule plan 1's experiment
// manifests follow, and for the same reason.
const caseFormatVersion = 1

// Case is one portable regression scenario: which fixture, at which seeds,
// against which firmware, and what must still be true.
type Case struct {
	FormatVersion int    `json:"format_version"`
	Name          string `json:"name"`
	// Fixture is a path or a built-in name, exactly as fixture.Load takes it.
	Fixture string   `json:"fixture"`
	Seeds   []uint64 `json:"seeds"`
	// RepeaterVersion and CompanionVersion pin firmware, overriding whatever
	// the fixture's own nodes carry. Empty leaves the fixture alone.
	RepeaterVersion  string `json:"repeater_version,omitempty"`
	CompanionVersion string `json:"companion_version,omitempty"`
	ForMs            uint32 `json:"for_ms"`
	// Assertions overrides the fixture's own, when given - a case exported
	// from a live run carries the claim that run demonstrated, which is not
	// necessarily every assertion the base fixture was authored with.
	Assertions []fixture.Assertion `json:"assertions,omitempty"`
	// Sends overrides the fixture's own traffic schedule, when given. A case
	// exported from a companion-driven sweep cannot reuse the fixture's own
	// schedule - the two are different stimuli - so it carries one of its
	// own, in the vocabulary this runner can actually replay: console
	// commands, not the companion protocol a sweep's send used.
	Sends []fixture.Send `json:"sends,omitempty"`
	// ToleranceBandPct widens an AssertDelivered-kind assertion's lower bound
	// before a miss counts as a hard failure rather than a flag - derived
	// from the observed seed spread of the run that created the case, never
	// chosen by hand: a band picked by hand is either too tight, and flakes,
	// or too loose, and stops meaning anything.
	ToleranceBandPct float64 `json:"tolerance_band_pct,omitempty"`
	// ExperimentID is the sweep or run this case was exported from, if any -
	// plan 1's ID, carried so a failure traces back to exactly the inputs
	// that produced it.
	ExperimentID string `json:"experiment_id,omitempty"`
}

// Verdict is the three-way outcome a stochastic metric needs and an
// invariant does not: a hard assertion fails outside its bound; an
// AssertDelivered-kind one only flags within its tolerance band, because a
// stochastic metric with too tight a band fails randomly and gets ignored
// if that counts as the same kind of red as a real regression.
type Verdict string

const (
	Pass Verdict = "pass"
	Flag Verdict = "flag"
	Fail Verdict = "fail"
)

// worse reports whether b outranks a on the pass < flag < fail order.
func worse(a, b Verdict) bool {
	rank := map[Verdict]int{Pass: 0, Flag: 1, Fail: 2}
	return rank[b] > rank[a]
}

// CheckResult is one assertion's evaluated verdict, softened by tolerance
// where the case says to.
type CheckResult struct {
	engine.Result
	Verdict Verdict
}

// SeedResult is one seed of one case.
type SeedResult struct {
	Seed    uint64
	Verdict Verdict
	Checks  []CheckResult
	// Delivered is the unique-delivery count this seed actually measured,
	// independent of whether any assertion asked about it - so a caller
	// calibrating a new case's threshold can read the real number without
	// parsing it back out of a Result's own prose.
	Delivered int
	Firmware  int
	Err       string
}

// CaseResult is a whole case: every seed it was run at, and the worst
// verdict among them - one seed failing makes the case fail, the same way
// one broken node makes a network's result the broken one, not the average.
type CaseResult struct {
	Case    Case
	Seeds   []SeedResult
	Verdict Verdict
	Took    time.Duration
}

// LoadCase reads a case file and refuses one this build cannot promise to
// run the way its author intended.
func LoadCase(path string) (Case, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Case{}, err
	}
	var c Case
	if err := json.Unmarshal(b, &c); err != nil {
		return Case{}, fmt.Errorf("%s is not a regression case: %w", path, err)
	}
	if c.FormatVersion == 0 {
		return Case{}, fmt.Errorf(
			"%s carries no format_version - it predates this runner and this "+
				"build cannot promise it still means what its author intended", path)
	}
	if c.FormatVersion > caseFormatVersion {
		return Case{}, fmt.Errorf(
			"%s is format version %d; this build only understands version %d and older",
			path, c.FormatVersion, caseFormatVersion)
	}
	if c.Fixture == "" {
		return Case{}, fmt.Errorf("%s names no fixture", path)
	}
	if len(c.Seeds) == 0 {
		return Case{}, fmt.Errorf("%s names no seeds", path)
	}
	if c.Name == "" {
		c.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return c, nil
}

// NewCase builds a case ready to save, deriving its tolerance band from the
// spread already observed rather than asking the caller to guess one.
func NewCase(name, fixturePath string, seeds []uint64, repeaterVersion, companionVersion string,
	forMs uint32, sends []fixture.Send, assertions []fixture.Assertion,
	observedSpreadPct float64, experimentID string) Case {
	return Case{
		FormatVersion: caseFormatVersion, Name: name, Fixture: fixturePath, Seeds: seeds,
		RepeaterVersion: repeaterVersion, CompanionVersion: companionVersion, ForMs: forMs,
		Sends: sends, Assertions: assertions, ToleranceBandPct: observedSpreadPct, ExperimentID: experimentID,
	}
}

// Save writes a case as indented JSON, the same shape LoadCase reads back.
func (c Case) Save(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, b, 0o644)
}

// pinFirmware overrides a node's firmware by role, the same rule
// session.withFirmware applies for a live sweep - duplicated rather than
// imported, because this package sits below session in the dependency
// order and pulling the GUI-adjacent package down to reach eight lines
// would cost more than it saves.
func pinFirmware(n scenario.Node, repeaterVersion, companionVersion string) scenario.Node {
	switch {
	case n.Kind == scenario.Companion && companionVersion != "":
		n.Firmware.Version = companionVersion
		n.Firmware.Role = "companion_radio"
	case n.Kind.RunsFirmware() && n.Kind != scenario.Companion && repeaterVersion != "":
		n.Firmware.Version = repeaterVersion
		n.Firmware.Role = "simple_repeater"
	}
	return n
}

// RunCase runs every seed of a case and reports the worst outcome. baseDir
// resolves a fixture path relative to the case file's own directory, so a
// directory of cases can be moved as a unit.
func RunCase(ctx context.Context, terr coverage.Terrain, c Case, baseDir string) (CaseResult, error) {
	began := time.Now()
	fxPath := c.Fixture
	if !filepath.IsAbs(fxPath) && baseDir != "" {
		if _, err := os.Stat(filepath.Join(baseDir, fxPath)); err == nil {
			fxPath = filepath.Join(baseDir, fxPath)
		}
	}
	fx, err := fixture.Load(fxPath)
	if err != nil {
		return CaseResult{}, fmt.Errorf("%s: %w", c.Name, err)
	}
	checks := fx.Checks()
	if len(c.Assertions) > 0 {
		checks = checks[:0]
		for _, a := range c.Assertions {
			checks = append(checks, engine.Assertion{
				Kind: engine.AssertKind(a.Kind), Node: a.Node, WithinMs: a.WithinMs,
				AtLeast: a.AtLeast, AtMost: a.AtMost, MaxPct: a.MaxPct,
			})
		}
	}
	if len(checks) == 0 {
		return CaseResult{}, fmt.Errorf("%s carries no assertions to check", c.Name)
	}

	forMs := c.ForMs
	if forMs == 0 {
		forMs = 120_000
	}

	result := CaseResult{Case: c, Verdict: Pass}
	for _, seed := range c.Seeds {
		sr := runSeed(ctx, terr, fx, checks, c.Sends, seed, c.RepeaterVersion, c.CompanionVersion,
			forMs, c.ToleranceBandPct)
		result.Seeds = append(result.Seeds, sr)
		if worse(result.Verdict, sr.Verdict) {
			result.Verdict = sr.Verdict
		}
	}
	result.Took = time.Since(began)
	return result, nil
}

func runSeed(ctx context.Context, terr coverage.Terrain, fx *fixture.Fixture,
	checks []engine.Assertion, sendsOverride []fixture.Send, seed uint64,
	repeaterVersion, companionVersion string, forMs uint32, toleranceBandPct float64) SeedResult {

	sf, bw, freq := RadioOf(fx)
	e := engine.New(terr, engine.Config{
		FreqMHz: freq, SF: sf, BandwidthHz: bw, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: seed,
	})
	defer func() { _ = e.Close() }()
	for _, n := range fx.Nodes {
		e.Add(pinFirmware(n, repeaterVersion, companionVersion), nil)
	}

	if err := e.AttachNative(ctx, seed); err != nil {
		return SeedResult{Seed: seed, Verdict: Fail, Err: err.Error()}
	}
	if err := Provision(e, fx); err != nil {
		return SeedResult{Seed: seed, Verdict: Fail, Err: err.Error()}
	}
	// The case's own stimulus, when it has one; otherwise the fixture's, and
	// failing that, a spread advert - the same fallback order cmd_check.go's
	// single-fixture runner uses, so a case with nothing scheduled is not a
	// case that measures nothing.
	sends := sendsOverride
	if len(sends) == 0 {
		sends = fx.Sends
	}
	if len(sends) == 0 {
		sends = AdvertSchedule(fx, 30_000)
	}
	if err := RunSends(ctx, e, sends, forMs); err != nil {
		return SeedResult{Seed: seed, Verdict: Fail, Err: err.Error()}
	}

	delivered := 0
	for _, sc := range e.Scoreboard() {
		delivered += sc.UniqueDelivery
	}
	results := e.Check(checks)
	sr := SeedResult{Seed: seed, Verdict: Pass, Firmware: e.FirmwareCount(), Delivered: delivered}
	for _, r := range results {
		v := classifyVerdict(r, toleranceBandPct)
		sr.Checks = append(sr.Checks, CheckResult{Result: r, Verdict: v})
		if worse(sr.Verdict, v) {
			sr.Verdict = v
		}
	}
	return sr
}

// classifyVerdict is the three-way rule itself: a passing result is Pass; a
// failing AssertDelivered within its tolerance band is Flag, because a
// stochastic count that varies with the seed is noise inside the band and a
// real regression outside it; anything else that fails is Fail, unsoftened -
// an invariant with no band to be inside of.
func classifyVerdict(r engine.Result, toleranceBandPct float64) Verdict {
	if r.Passed {
		return Pass
	}
	if r.Assertion.Kind == engine.AssertDelivered && toleranceBandPct > 0 {
		widened := int(float64(r.Assertion.AtLeast) * (1 - toleranceBandPct/100))
		if widened < 0 {
			widened = 0
		}
		if deliveredFrom(r.Detail) >= widened {
			return Flag
		}
	}
	return Fail
}

// deliveredFrom pulls the measured count back out of a Result's own detail
// string ("%d unique deliveries, wanted at least %d") rather than
// re-measuring it - Result is the one place that count already lives.
func deliveredFrom(detail string) int {
	var n int
	_, _ = fmt.Sscanf(detail, "%d unique deliveries", &n)
	return n
}

// RunDir runs every *.json case in a directory and returns them sorted by
// name, so a report reads the same way twice.
func RunDir(ctx context.Context, terr coverage.Terrain, dir string) ([]CaseResult, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []error{err}
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var cases []Case
	var errs []error
	for _, name := range names {
		c, err := LoadCase(filepath.Join(dir, name))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		cases = append(cases, c)
	}
	results, runErrs := RunCases(ctx, terr, cases, dir)
	return results, append(errs, runErrs...)
}

// RunCases runs an already-loaded list of cases, in order, and returns them
// alongside any that failed to run - the part RunDir shares with a caller
// that built its cases in Go rather than reading them off disk (the
// pathological suite, bundled in the binary rather than on a filesystem a
// fresh install has no guarantee of).
func RunCases(ctx context.Context, terr coverage.Terrain, cases []Case, baseDir string) ([]CaseResult, []error) {
	var results []CaseResult
	var errs []error
	for _, c := range cases {
		r, err := RunCase(ctx, terr, c, baseDir)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		results = append(results, r)
	}
	return results, errs
}
