// Is the model still telling the truth?
//
// The chain ADR-0015 describes: fetch what really happened, replay it here,
// compare. The calibration that comes out of it is the excess path loss term -
// the same one whose absence made links cross the Lomond ridge - so this is
// where a number to put in it comes from rather than a guess.
package study

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/provider"
)

// minValidateHours and maxValidateHours bound the window of observations a
// validation run asks a deployment for. A minute is the shortest window a
// public feed has anything in; a year is longer than any of them keeps.
const (
	minValidateHours = 1.0 / 60
	maxValidateHours = 24 * 365
)

func registerValidate(st *state.Store, s *session.Sim) {
	st.HandleSpec("validate.fetch", state.Spec{
		What: "pull what a real deployment actually heard and hand it to the " +
			"comparison, which is where a number for the excess loss term comes " +
			"from rather than a guess",
		Params: []state.Param{
			{Name: "url", Type: state.ParamString, Primary: true,
				What: "the CoreScope base URL to ask; absent falls back to the " +
					"source set with import.set_source, and is refused when " +
					"there is no source either way"},
			{Name: "hours", Type: state.ParamNumber,
				What: "how far back to ask for, from a minute to a year; " +
					"absent is a day, and a value outside that range or not a " +
					"number is refused rather than quietly made a day"},
		},
		Returns: []string{"fetching", "hours"},
		Answers: "It answers the moment the fetch starts, not with anything " +
			"fetched. The comparison lands later through the internal " +
			"`validate.compare`, and every way it can come to nothing - neither " +
			"endpoint answering, observations carrying keys from another " +
			"network, no SNR in the window - through `validate.failed` on the " +
			"status line. This is the one verb here that goes to the network.",
		Example: &state.Example{
			Params: map[string]any{"url": "https://corescope.example/", "hours": 24},
			What:   "fetch a day of real receptions to compare against",
		},
	}, func(w *state.World, p any) (any, error) {
		url, _ := session.StringField(p, "url")
		if url == "" && s.ImportURL() != "" {
			url = s.ImportURL()
		}
		if url == "" {
			return nil, fmt.Errorf("no source: set one with import.set_source")
		}
		url = strings.TrimRight(url, "/")
		// A day when nobody says otherwise, and a refusal when somebody says
		// something unusable. Zero, a negative and the string "twelve" all used
		// to come out as twenty-four, so the report said it had compared a
		// day's observations and the caller believed it had asked for one.
		hours, err := session.NumInRange("validate.fetch", "hours", p, 24,
			minValidateHours, maxValidateHours)
		if err != nil {
			return nil, err
		}
		id := "validate"
		w.Jobs = append(w.Jobs, state.Job{
			ID: id, What: "fetching observations", Total: 1})
		go func() {
			cs := &provider.CoreScope{BaseURL: url}
			since := time.Now().Add(-time.Duration(hours) * time.Hour)
			// The receptions endpoint first, because where it exists it is
			// exactly this question. Where it does not - a live CoreScope
			// answers /api/receptions with its own HTML - the packets
			// endpoint carries the same evidence: an observer, a hop count
			// and the SNR it heard.
			recs, err := cs.Receptions(context.Background(), since)
			if err == nil && len(recs) > 0 {
				_, _ = st.Do(context.Background(), "validate.compare", recs)
				return
			}
			pkts, perr := cs.Packets(context.Background(), 4000, nil)
			if perr != nil {
				if err == nil {
					err = perr
				}
				_, _ = st.Do(context.Background(), "validate.failed",
					"neither endpoint answered: "+err.Error())
				return
			}
			// Names, not keys.
			//
			// CoreScope resolves paths to public keys, and an advert's sender
			// is a key in its own payload. The scenario's nodes keep the real
			// key they were imported with, so the map exists; it just was not
			// being made - which is why 3,989 observations once matched
			// nothing at all.
			byKey := map[string]string{}
			names := map[string]bool{}
			for _, n := range s.Nodes() {
				if k := strings.ToLower(n.PublicKey); k != "" {
					byKey[k] = n.Name
				}
				names[n.Name] = true
			}
			named := func(key string) string {
				k := strings.ToLower(key)
				if name, ok := byKey[k]; ok {
					return name
				}
				// A prefix is enough: MeshCore paths carry the first bytes of
				// a key, and a source that resolves them may hand back either.
				for full, name := range byKey {
					if len(k) >= 8 && strings.HasPrefix(full, k) {
						return name
					}
				}
				// Some deployments resolve paths all the way to names.
				if names[key] {
					return key
				}
				return ""
			}

			var out []provider.Reception
			unresolved := 0
			for _, p := range pkts {
				if !p.HasSNR || p.At.Before(since) || p.Receiver == "" {
					continue
				}
				// The SNR belongs to whoever transmitted the copy that was
				// heard - the last relay on the path, or the origin itself
				// when the path is empty. Not the packet's origin: a message
				// three relays deep says nothing about the link between its
				// origin and this observer, and pairing those two was one
				// half of how a calibration run matched nothing.
				tx := named(p.Sender)
				if tx == "" {
					unresolved++
					continue
				}
				r := provider.Reception{
					At: p.At, Receiver: p.Receiver, Transmitter: tx,
					PacketID: p.PacketID,
					HopCount: len(p.PathHashes),
					HasSNR:   true, SNRdB: p.SNRdB,
				}
				if r.HopCount == 0 {
					r.Origin = tx
				}
				out = append(out, r)
			}
			if len(out) == 0 && unresolved > 0 {
				_, _ = st.Do(context.Background(), "validate.failed", fmt.Sprintf(
					"%d observations carried SNR but none named a node in this "+
						"scenario: the keys they carry belong to a different network",
					unresolved))
				return
			}
			if len(out) == 0 {
				_, _ = st.Do(context.Background(), "validate.failed",
					fmt.Sprintf("%d packets carried no SNR from a named observer in that window",
						len(pkts)))
				return
			}
			// Reported, not discarded: an error returned to a goroutine
			// nobody reads is a step that silently did not happen, and the
			// next thing to run then calibrates against nothing.
			if _, err := st.Do(context.Background(), "validate.compare", out); err != nil {
				_, _ = st.Do(context.Background(), "validate.failed", err.Error())
			}
		}()
		return map[string]any{"fetching": true, "hours": hours}, nil
	})

	st.HandleInternalSpec("validate.failed", state.Spec{
		What: "take the validation job off the status line and put the reason " +
			"there instead, so a fetch that matched nothing says which of the " +
			"several nothings it was",
		Answers: "Answers nothing.",
	}, func(w *state.World, p any) (any, error) {
		msg := session.SoleString(p)
		w.Jobs = session.FinishJob(w.Jobs, "validate")
		w.Say("validate: " + msg)
		return nil, nil
	})

	st.HandleInternalSpec("validate.compare", state.Spec{
		What: "put what the model predicted beside what was actually heard, and " +
			"turn the difference into a total excess loss worth applying",
		Returns: []string{"matched", "unmatched", "median_db", "iqr_db",
			"suggested_excess_loss_db"},
		Answers: "A positive `median_db` means the model predicted a stronger " +
			"signal than was heard, which is the simulator being kinder than " +
			"the air. `suggested_excess_loss_db` is a total and not a delta: " +
			"the links these residuals were measured against already carried " +
			"the current term, so it is that term plus the median. Saturated " +
			"observations are counted but do not vote. It refuses when nothing " +
			"matched, and says how much of that was names outside the scenario " +
			"against pairs with no measured link, because the two want " +
			"different fixes.",
	}, func(w *state.World, p any) (any, error) {
		recs, ok := p.([]provider.Reception)
		if !ok {
			return nil, session.WrongCallback("validate.compare")
		}
		w.Jobs = session.FinishJob(w.Jobs, "validate")
		if len(recs) == 0 {
			return nil, fmt.Errorf("no observations in that window")
		}
		obs := observedFrom(recs)
		w.Observed = obs
		res := s.ResidualsOf(obs, w.Links, w.Nodes)
		w.Residuals = res
		if res == nil || res.Matched == 0 {
			// The split is the diagnosis: names outside the scenario mean the
			// import does not cover the network observed; pairs with no link
			// mean the engine has not measured them yet. Their sum looking
			// like one number is how this failure went unexplained.
			detail := ""
			if res != nil {
				detail = fmt.Sprintf(": %d named a node this scenario does not have, "+
					"%d had no measured link between their pair - "+
					"import the region the observers cover, then let the links warm",
					res.OffScenario, res.NoLink)
			}
			return nil, fmt.Errorf(
				"none of the %d observations matched a pair in this scenario%s",
				len(recs), detail)
		}
		// The sign convention, stated: positive means the model predicted a
		// stronger signal than was heard, so the model is optimistic and the
		// excess loss term should go up.
		w.Say(fmt.Sprintf("%d observations matched (%d saturated, not voting), "+
			"median residual %+.1f dB (positive means the model is optimistic)",
			res.Matched, res.Censored, res.MedianDB))
		return map[string]any{
			"matched": res.Matched, "unmatched": res.Unmatched,
			"median_db": res.MedianDB, "iqr_db": res.IQRdB,
			// The suggestion is a total, not a delta. The links these
			// residuals were measured against already carried the current
			// excess loss, so the median is what *remains* on top of it -
			// suggesting the median alone would tell an operator to replace
			// 20 dB of measured clutter with 6 dB of leftover bias, and the
			// model would come out 14 dB more optimistic for having been
			// calibrated.
			"suggested_excess_loss_db": maxFloat(0, s.ExcessLossDB()+res.MedianDB),
		}, nil
	})

	st.HandleSpec("validate.calibrate", state.Spec{
		What: "set the excess path loss term from what the comparison measured, " +
			"or from a stated figure, and rebuild every link on it",
		Params: []state.Param{
			{Name: "db", Type: state.ParamNumber, Primary: true,
				What: "the excess loss to apply, in decibels, which wins over " +
					"the measurement; absent uses the measured residual, a " +
					"negative is refused because excess loss is a loss, and a " +
					"db that cannot be read as a number is refused rather than " +
					"falling back to the residual nobody asked for"},
		},
		Returns: []string{"db", "links"},
		Answers: "Refuses when nothing has been measured and no db was given: " +
			"defaulting to 0 dB there is not the absence of calibration but the " +
			"most optimistic model there is. The figure applied is a total on " +
			"top of nothing, not a delta, so repeated fetch-then-calibrate " +
			"rounds converge. It rebuilds the engine and re-measures every " +
			"link, so `links` is 0 at the moment it answers and fills in as the " +
			"matrix warms.",
		Example: &state.Example{
			Params: map[string]any{"db": 12},
			What:   "apply a stated excess loss rather than a measured one",
		},
	}, func(w *state.World, p any) (any, error) {
		db, have := 0.0, false
		if w.Residuals != nil && w.Residuals.Matched > 0 {
			// On top of the current term, because that is what the residuals
			// measured: the links they were compared against already carried
			// s.ExcessLossDB(), so the median is the bias that term did not
			// cover. Setting the total *to* the median - which this once did -
			// silently discarded the existing calibration and made a
			// calibrated model more optimistic than an uncalibrated one.
			// Repeated fetch-then-calibrate rounds converge: each fit is of
			// what the previous round left over.
			db, have = maxFloat(0, s.ExcessLossDB()+w.Residuals.MedianDB), true
		}
		// A db that was given wins over the measurement. A db that was given and
		// cannot be read is refused: falling back to the residuals would apply
		// a number the caller never asked for, under a verb whose whole job is
		// to set that number deliberately.
		if v, asked, err := session.NumAsked("validate.calibrate", "db", p); err != nil {
			return nil, err
		} else if asked {
			db, have = v, true
		}
		// Refuse rather than default. Called with nothing measured this used
		// to apply 0 dB, which is not "no calibration" - it is the most
		// optimistic model there is, and it silently put back every link that
		// crosses a ridge.
		if !have {
			return nil, fmt.Errorf(
				"nothing has been measured yet: fetch observations first, or give a db")
		}
		if db < 0 {
			return nil, fmt.Errorf("excess loss is a loss: %.1f dB would add signal", db)
		}
		s.SetExcessLoss(db, true)
		w.ExcessLossDB, w.Calibrated = db, true
		if len(s.Nodes()) > 0 {
			if err := s.Rebuild(w); err != nil {
				return nil, err
			}
			w.Links = nil
			s.Warm(st, len(s.Nodes()))
		}
		w.Say(fmt.Sprintf("excess path loss calibrated to %.1f dB; measuring links", db))
		return map[string]any{"db": db, "links": len(w.Links)}, nil
	})

	st.HandleSpec("validate.uncalibrate", state.Spec{
		What: "put the excess path loss back to the default, which is a stated " +
			"guess rather than a measurement, and rebuild every link on it",
		Returns: []string{"db"},
		Answers: "The snapshot stops calling itself calibrated, which is the " +
			"point: the default is a reasonable figure for typical clutter and " +
			"not this network's. It re-measures the whole link matrix, so the " +
			"margins move over the seconds after it answers.",
		Example: &state.Example{
			Params: map[string]any{},
			What:   "drop a calibration and go back to the stated default",
		},
	}, func(w *state.World, _ any) (any, error) {
		s.SetExcessLoss(session.DefaultExcessLossDB, false)
		w.ExcessLossDB, w.Calibrated = session.DefaultExcessLossDB, false
		if len(s.Nodes()) > 0 {
			if err := s.Rebuild(w); err != nil {
				return nil, err
			}
			w.Links = nil
			s.Warm(st, len(s.Nodes()))
		}
		w.Say(fmt.Sprintf("excess path loss back to the %.1f dB default", float64(session.DefaultExcessLossDB)))
		return map[string]any{"db": session.DefaultExcessLossDB}, nil
	})
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// observedFrom turns provider receptions into the interface's own shape.
func observedFrom(recs []provider.Reception) []state.Observed {
	out := make([]state.Observed, 0, len(recs))
	for _, r := range recs {
		out = append(out, state.Observed{
			At: r.At, Receiver: r.Receiver, Origin: r.Origin,
			Transmitter: r.Transmitter,
			HopCount:    r.HopCount, HasSNR: r.HasSNR, SNRdB: r.SNRdB,
			PacketID: r.PacketID,
		})
	}
	return out
}
