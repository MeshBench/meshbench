package main

import (
	"context"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/MeshBench/meshbench/internal/regression"
)

// runVerify is the directory runner: every regression case in a directory,
// run on real firmware, reported pass, fail or flag - "will this stay
// fixed?", answered by a machine rather than by re-reading two event logs.
//
// Non-zero exit only on a hard failure. A flagged case - a stochastic metric
// outside its tolerance band - is reported but does not fail the build: a
// gate that treats noise as a regression gets ignored, which is worse than
// no gate at all.
func runVerify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	store := terrainFlags(fs)
	junit := fs.String("junit", "", "write a JUnit XML report here")
	quiet := fs.Bool("quiet", false, "only print the summary and failures")
	if err := parse(fs, args, "run a directory of regression scenarios and check their assertions"); err != nil {
		return err
	}
	dir := fs.Arg(0)
	if dir == "" {
		return fmt.Errorf("give a directory of regression cases: meshcoresim verify ./regressions")
	}
	t, err := store()
	if err != nil {
		return err
	}

	began := time.Now()
	results, errs := regression.RunDir(ctx, t, dir)
	for _, e := range errs {
		// A malformed case is refused by name, not silently skipped - a
		// directory that quietly ran 40 of 41 scenarios is not the report
		// its author asked for.
		fmt.Fprintln(os.Stderr, "meshcoresim:", e)
	}
	if len(results) == 0 && len(errs) == 0 {
		return fmt.Errorf("%s has no regression cases (*.json)", dir)
	}

	var passed, flagged, failed int
	for _, r := range results {
		switch r.Verdict {
		case regression.Pass:
			passed++
		case regression.Flag:
			flagged++
		case regression.Fail:
			failed++
		}
		if *quiet && r.Verdict == regression.Pass {
			continue
		}
		printCase(r)
	}
	fmt.Printf("\n%d scenarios: %d passed, %d failed, %d flagged, %v\n",
		len(results), passed, failed, flagged, time.Since(began).Round(time.Second))

	if *junit != "" {
		if err := writeVerifyJUnit(*junit, results); err != nil {
			return err
		}
		fmt.Printf("JUnit written to %s\n", *junit)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d regression case(s) could not be run", len(errs))
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d scenarios failed", failed, len(results))
	}
	return nil
}

func printCase(r regression.CaseResult) {
	mark := map[regression.Verdict]string{
		regression.Pass: "pass", regression.Flag: "flag", regression.Fail: "FAIL",
	}[r.Verdict]
	fmt.Printf("%-4s %-32s %d seed(s), %v\n", mark, r.Case.Name, len(r.Seeds), r.Took.Round(time.Second))
	if r.Verdict == regression.Pass {
		return
	}
	for _, sr := range r.Seeds {
		if sr.Verdict == regression.Pass {
			continue
		}
		if sr.Err != "" {
			fmt.Printf("       seed %d: %s\n", sr.Seed, sr.Err)
			continue
		}
		for _, c := range sr.Checks {
			if c.Verdict == regression.Pass {
				continue
			}
			fmt.Printf("       seed %d %-4s %s: %s\n",
				sr.Seed, c.Verdict, c.Assertion.String(), c.Detail)
		}
	}
}

// JUnit for a directory: one testcase per case, one suite for the run - the
// same subset cmd_check.go's single-fixture report writes, so one pipeline
// step reads either. A flag counts as a skip, not a failure, so CI's own
// summary matches the exit code's own leniency toward them.
type verifySuite struct {
	XMLName  xml.Name         `xml:"testsuite"`
	Name     string           `xml:"name,attr"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Skipped  int              `xml:"skipped,attr"`
	Time     float64          `xml:"time,attr"`
	Cases    []verifyJUnitRow `xml:"testcase"`
}

type verifyJUnitRow struct {
	Name    string        `xml:"name,attr"`
	Time    float64       `xml:"time,attr"`
	Failure *verifyDetail `xml:"failure,omitempty"`
	Skip    *verifyDetail `xml:"skipped,omitempty"`
}

type verifyDetail struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

func writeVerifyJUnit(path string, results []regression.CaseResult) error {
	suite := verifySuite{Name: "regressions", Tests: len(results)}
	for _, r := range results {
		row := verifyJUnitRow{Name: r.Case.Name, Time: r.Took.Seconds()}
		detail := verdictDetail(r)
		switch r.Verdict {
		case regression.Fail:
			suite.Failures++
			row.Failure = &verifyDetail{Message: detail, Text: detail}
		case regression.Flag:
			suite.Skipped++
			row.Skip = &verifyDetail{Message: detail, Text: detail}
		}
		suite.Cases = append(suite.Cases, row)
		suite.Time += r.Took.Seconds()
	}
	b, err := xml.MarshalIndent(suite, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(xml.Header), b...), 0o644)
}

func verdictDetail(r regression.CaseResult) string {
	for _, sr := range r.Seeds {
		if sr.Err != "" {
			return fmt.Sprintf("seed %d: %s", sr.Seed, sr.Err)
		}
		for _, c := range sr.Checks {
			if c.Verdict != regression.Pass {
				return fmt.Sprintf("seed %d, %s: %s", sr.Seed, c.Assertion.String(), c.Detail)
			}
		}
	}
	return string(r.Verdict)
}
