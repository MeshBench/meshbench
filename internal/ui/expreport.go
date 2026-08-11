package ui

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// exportExperiment writes the sweep as a self-contained page.
//
// The verdict first, because "did it make a difference" is the question the
// experiment was run to answer and a table of averages does not answer it — a
// reader has to do the arithmetic and decide for themselves whether a 4%
// difference beats the noise, which is exactly where a wrong conclusion gets
// made. When the answer is no, the investigation goes in its place: a parameter
// that changed nothing because it never reached the firmware looks identical, in
// the numbers, to one that genuinely does not matter.
//
// Self-contained SVG and no external assets, so it opens anywhere and can be
// pulled into a write-up without carrying a directory of images with it.
func (a *App) exportExperiment(e *experiment) (string, error) {
	dir := filepath.Join(projectDir(), "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("experiment-%s.html",
		time.Now().Format("2006-01-02-150405")))

	v := a.verdictFor(e)
	sums := e.summarise()

	var b strings.Builder
	b.WriteString(`<!doctype html><meta charset="utf-8"><title>MeshBench experiment</title>
<style>
:root{--ink:#14181c;--dim:#5a656d;--rule:#d8d9d4;--ground:#f7f7f4;--card:#fff;
--ok:#40682c;--bad:#a1442a;--accent:#1f6f7d}
@media (prefers-color-scheme:dark){:root{--ink:#e6e8e6;--dim:#98a3ab;--rule:#28313a;
--ground:#0e1216;--card:#161c22;--ok:#93bd6b;--bad:#e08a6f;--accent:#5fb3c4}}
*{box-sizing:border-box}
body{background:var(--ground);color:var(--ink);margin:0;padding:32px 24px 80px;
font:16px/1.6 Charter,Georgia,serif}
.wrap{max-width:1000px;margin:0 auto}
h1{font:600 30px/1.15 ui-monospace,Menlo,monospace;margin:0 0 6px;letter-spacing:-.02em}
.sub{color:var(--dim);margin:0 0 28px}
h2{font:600 12px/1 ui-monospace,Menlo,monospace;letter-spacing:.16em;text-transform:uppercase;
color:var(--accent);margin:40px 0 10px;padding-bottom:8px;border-bottom:1px solid var(--rule)}
.verdict{background:var(--card);border:1px solid var(--rule);border-left:4px solid var(--bad);
padding:20px 22px;margin-bottom:8px}
.verdict.yes{border-left-color:var(--ok)}
.verdict p{margin:0;font-size:19px}
.verdict ul{margin:14px 0 0;padding-left:20px;color:var(--dim);font-size:15px}
.verdict li{margin-bottom:8px}
table{border-collapse:collapse;width:100%;font:14px ui-monospace,Menlo,monospace;
font-variant-numeric:tabular-nums}
th{text-align:left;font-size:11px;letter-spacing:.1em;text-transform:uppercase;color:var(--dim);
padding:0 12px 8px 0;border-bottom:1px solid var(--rule)}
td{padding:8px 12px 8px 0;border-bottom:1px solid var(--rule)}
.up{color:var(--ok)}.down{color:var(--bad)}.flag{color:var(--bad)}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));gap:16px;margin-top:16px}
.cell{background:var(--card);border:1px solid var(--rule);padding:12px 14px}
.cell h3{margin:0 0 8px;font:600 13px ui-monospace,Menlo,monospace}
.scroll{overflow-x:auto}
footer{margin-top:56px;padding-top:16px;border-top:1px solid var(--rule);
color:var(--dim);font:12px ui-monospace,Menlo,monospace}
</style><div class="wrap">`)

	fmt.Fprintf(&b, "<h1>%s</h1>", html.EscapeString(experimentTitle(e)))
	fmt.Fprintf(&b, `<p class="sub">%d arms · %d seeds · %d runs · %s</p>`,
		len(e.Arms), len(e.Seeds), len(e.results), time.Now().Format("2 January 2006, 15:04"))

	cls := ""
	if v.Difference {
		cls = " yes"
	}
	fmt.Fprintf(&b, `<h2>Verdict</h2><div class="verdict%s"><p>%s</p>`, cls, html.EscapeString(v.Headline))
	if len(v.Investigation) > 0 {
		b.WriteString("<ul>")
		for _, l := range v.Investigation {
			fmt.Fprintf(&b, "<li>%s</li>", html.EscapeString(l))
		}
		b.WriteString("</ul>")
	}
	b.WriteString("</div>")
	if w := e.notAResultYet(); w != "" {
		fmt.Fprintf(&b, `<p class="flag">! %s</p>`, html.EscapeString(w))
	}

	// The matrix, as deltas from the baseline.
	b.WriteString(`<h2>Matrix</h2><div class="scroll"><table><tr>
<th>arm</th><th>runs</th><th>tx</th><th>rx</th><th>to repeaters</th><th>to companions</th>
<th>collisions</th><th>deaf</th><th>airtime</th><th>to quiet</th></tr>`)
	if len(sums) > 0 {
		base := sums[0]
		if e.baselineArm < len(sums) {
			base = sums[e.baselineArm]
		}
		for i, s := range sums {
			fmt.Fprintf(&b, "<tr><td>%s</td><td>%d%s</td>", html.EscapeString(s.Arm), s.Runs,
				flaggedNote(s.Flagged))
			for _, m := range []struct{ v, ref float64 }{
				{s.TX, base.TX}, {s.RX, base.RX}, {s.RepPct, base.RepPct}, {s.CompPct, base.CompPct},
				{s.Coll, base.Coll}, {s.Deaf, base.Deaf},
				{s.Airtime / 1000, base.Airtime / 1000}, {s.SpanMs / 1000, base.SpanMs / 1000},
			} {
				if i == e.baselineArm || m.ref == 0 {
					fmt.Fprintf(&b, "<td>%.0f</td>", m.v)
					continue
				}
				d := (m.v - m.ref) / m.ref * 100
				k := ""
				if d > 0.5 {
					k = ` class="up"`
				} else if d < -0.5 {
					k = ` class="down"`
				}
				fmt.Fprintf(&b, "<td%s>%+.1f%%</td>", k, d)
			}
			b.WriteString("</tr>")
		}
	}
	b.WriteString("</table></div>")

	// Small multiples: one flood shape per arm, shared axis.
	b.WriteString(`<h2>Receptions per second, by arm</h2><div class="grid">`)
	byArm, order, peak := armSeries(e)
	for _, arm := range order {
		fmt.Fprintf(&b, `<div class="cell"><h3>%s</h3>%s</div>`,
			html.EscapeString(arm), sparkSVG(byArm[arm], peak))
	}
	b.WriteString("</div>")

	// Every run, so nothing is hidden behind an average.
	b.WriteString(`<h2>Runs</h2><div class="scroll"><table><tr>
<th>arm</th><th>seed</th><th>tx</th><th>rx</th><th>messages</th><th>repeaters + companions, each message</th>
<th>collisions</th><th>to quiet</th><th>note</th></tr>`)
	for _, r := range e.results {
		note := ""
		if r.Err != "" {
			note = r.Err
		} else if r.Flag != "" {
			note = r.Flag
		}
		// Every message's own reach, not just the mean: six senders is six
		// numbers, and one that got nowhere matters more than the average does.
		var each []string
		for i := range r.RepPerMsg {
			who := ""
			if i < len(r.SenderOf) {
				who = r.SenderOf[i]
			}
			each = append(each, fmt.Sprintf(`<span title="from %s">%d + %d</span>`,
				html.EscapeString(who), r.RepPerMsg[i], r.CompPerMsg[i]))
		}
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td>
<td>%s</td><td>%d</td><td>%.1f s</td><td class="flag">%s</td></tr>`,
			html.EscapeString(r.Arm), r.Seed, r.TX, r.RX, r.Messages,
			strings.Join(each, " · "), r.Collisions,
			float64(r.SpanMs)/1000, html.EscapeString(note))
	}
	b.WriteString("</table></div>")

	// First divergence between the first two arms at the first seed: the
	// question a table of totals cannot answer.
	if len(e.Arms) >= 2 && len(e.Seeds) >= 1 {
		x := findResult(e.results, e.Arms[0].Label, e.Seeds[0])
		y := findResult(e.results, e.Arms[1].Label, e.Seeds[0])
		if x != nil && y != nil {
			fmt.Fprintf(&b, `<h2>First divergence</h2><p class="sub">%s against %s, seed %d</p><pre>%s</pre>`,
				html.EscapeString(x.Arm), html.EscapeString(y.Arm), e.Seeds[0],
				html.EscapeString(firstDivergence(x.ledger, y.ledger)))
		}
	}

	fmt.Fprintf(&b, `<footer>MeshBench · %d nodes · senders %s · %s on %s at %.0f s, measured for %.0f s`+
		`<br>results are a best case: no multipath, bare earth, ideal demodulator</footer></div>`,
		len(a.Nodes), html.EscapeString(strings.Join(e.Senders, ", ")),
		html.EscapeString(e.Channel), html.EscapeString(e.Scope),
		float64(e.SendAtMs)/1000, float64(e.RunForMs)/1000)

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func experimentTitle(e *experiment) string {
	if len(e.Arms) == 0 {
		return "MeshBench experiment"
	}
	return "Experiment: " + e.Arms[0].Label + " and " + fmt.Sprint(len(e.Arms)-1) + " more"
}

func flaggedNote(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(` <span class="flag">(%d flagged)</span>`, n)
}

func armSeries(e *experiment) (map[string][]int, []string, int) {
	byArm := map[string][]int{}
	var order []string
	peak := 1
	for _, r := range e.results {
		if _, seen := byArm[r.Arm]; !seen {
			order = append(order, r.Arm)
			byArm[r.Arm] = make([]int, len(r.perSecond))
		}
		for i, v := range r.perSecond {
			if i < len(byArm[r.Arm]) {
				byArm[r.Arm][i] += v
				if byArm[r.Arm][i] > peak {
					peak = byArm[r.Arm][i]
				}
			}
		}
	}
	return byArm, order, peak
}

// sparkSVG draws one arm's flood shape. Bars rather than a line: the quantity
// is a count per second, and a line implies a continuity it does not have.
func sparkSVG(series []int, peak int) string {
	if len(series) == 0 || peak <= 0 {
		return "<p>no data</p>"
	}
	const w, h = 300.0, 70.0
	bw := w / float64(len(series))
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %g %g" width="100%%" height="%g" role="img">`, w, h+14, h+14)
	fmt.Fprintf(&b, `<line x1="0" y1="%g" x2="%g" y2="%g" stroke="currentColor" stroke-opacity=".25"/>`, h, w, h)
	for i, v := range series {
		bh := float64(v) / float64(peak) * (h - 2)
		if v > 0 && bh < 1 {
			bh = 1
		}
		fmt.Fprintf(&b, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" fill="currentColor" fill-opacity=".55"/>`,
			float64(i)*bw, h-bh, bw*0.82, bh)
	}
	fmt.Fprintf(&b, `<text x="0" y="%g" font-size="9" fill="currentColor" fill-opacity=".6">0 s</text>`, h+11)
	fmt.Fprintf(&b, `<text x="%g" y="%g" font-size="9" fill="currentColor" fill-opacity=".6">%d s</text>`,
		w-24, h+11, len(series))
	b.WriteString("</svg>")
	return b.String()
}
