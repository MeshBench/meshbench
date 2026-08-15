// Comment formats a run of regression cases as one GitHub PR comment: a
// one-line summary always, and only the scenarios that diverged expanded -
// "a bot that writes an essay gets muted." A clean run updates in place
// rather than adding noise; the caller is the one deciding whether to post
// at all, this only decides what the words say.
package regression

import (
	"fmt"
	"sort"
	"strings"
)

// Marker is the HTML comment every comment this package writes carries, so
// a caller can find and update its own previous comment rather than
// growing a new one on every push.
const Marker = "<!-- meshbench-regression-check -->"

// Comment renders results as GitHub-flavoured markdown. checkName labels the
// summary line (a MeshCore build ref is the usual choice, so the reader
// knows what was actually built).
func Comment(results []CaseResult, checkName string) string {
	var passed, flagged, failed int
	var totalSeeds int
	for _, r := range results {
		switch r.Verdict {
		case Pass:
			passed++
		case Flag:
			flagged++
		case Fail:
			failed++
		}
		totalSeeds += len(r.Seeds)
	}

	var b strings.Builder
	b.WriteString(Marker)
	b.WriteString("\n### MeshBench regression check")
	if checkName != "" {
		fmt.Fprintf(&b, " — %s", checkName)
	}
	b.WriteString("\n\n")

	if failed == 0 && flagged == 0 {
		fmt.Fprintf(&b, "**%d scenarios passed.** No regression against the baseline these cases were captured from.\n",
			passed)
		return b.String()
	}

	fmt.Fprintf(&b, "%d scenarios: **%d passed**, **%d regression%s**, **%d flagged**",
		len(results), passed, failed, plural(failed), flagged)
	if len(results) > 0 {
		fmt.Fprintf(&b, " (seeds each %d)", totalSeeds/len(results))
	}
	b.WriteString("\n\n")

	// Only the diverging scenarios are expanded. Sorted worst-first, so a
	// reviewer's eye lands on what needs it before what merely flagged.
	diverged := make([]CaseResult, 0, failed+flagged)
	for _, r := range results {
		if r.Verdict != Pass {
			diverged = append(diverged, r)
		}
	}
	sort.SliceStable(diverged, func(i, j int) bool {
		return worse(diverged[j].Verdict, diverged[i].Verdict)
	})
	for _, r := range diverged {
		writeCaseDetail(&b, r)
	}

	if passed > 0 {
		fmt.Fprintf(&b, "\n%d other scenario%s passed and %s collapsed here.\n",
			passed, plural(passed), pronoun(passed))
	}
	return b.String()
}

func writeCaseDetail(b *strings.Builder, r CaseResult) {
	label := "regression"
	if r.Verdict == Flag {
		label = "flagged — within tolerance"
	}
	fmt.Fprintf(b, "<details>\n<summary><b>%s</b> — %s</summary>\n\n", r.Case.Name, label)
	for _, sr := range r.Seeds {
		if sr.Verdict == Pass {
			continue
		}
		if sr.Err != "" {
			fmt.Fprintf(b, "- seed %d: %s\n", sr.Seed, sr.Err)
			continue
		}
		for _, c := range sr.Checks {
			if c.Verdict == Pass {
				continue
			}
			fmt.Fprintf(b, "- seed %d, `%s`: %s\n", sr.Seed, c.Verdict, c.Detail)
		}
	}
	if r.Case.ExperimentID != "" {
		fmt.Fprintf(b, "\nreproduce: `meshcoresim verify %s.json`  (experiment `%s`)\n",
			r.Case.Name, r.Case.ExperimentID)
	} else {
		fmt.Fprintf(b, "\nreproduce: `meshcoresim verify %s.json`\n", r.Case.Name)
	}
	b.WriteString("</details>\n\n")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func pronoun(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
