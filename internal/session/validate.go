// Is the model still telling the truth?
//
// The chain ADR-0015 describes: fetch what really happened, replay it here,
// compare. The calibration that comes out of it is the excess path loss term -
// the same one whose absence made links cross the Lomond ridge - so this is
// where a number to put in it comes from rather than a guess.
package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/provider"
)

func registerValidate(st *state.Store, s *Sim) {
	// validate.fetch: real receptions, replayed against the model.
	st.Handle("validate.fetch", func(w *state.World, p any) (any, error) {
		url, _ := stringField(p, "url")
		if url == "" && s.imp != nil {
			url = s.imp.url
		}
		if url == "" {
			return nil, fmt.Errorf("no source: set one with import.set_source")
		}
		url = strings.TrimRight(url, "/")
		hours := 24.0
		if v, ok := numField(p, "hours"); ok && v > 0 {
			hours = v
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
			// CoreScope's packet records carry no origin field, and the path
			// it resolves is a list of public keys. Every observation
			// therefore names a pair that no scenario recognises - which is
			// why 3,989 of them matched nothing at all. The scenario's own
			// nodes keep the real key they were imported with, so the map
			// exists; it just was not being made.
			byKey := map[string]string{}
			for _, n := range s.nodes {
				if k := strings.ToLower(n.PublicKey); k != "" {
					byKey[k] = n.Name
				}
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
				return ""
			}

			var out []provider.Reception
			unresolved := 0
			for _, p := range pkts {
				if !p.HasSNR || p.At.Before(since) || p.Receiver == "" {
					continue
				}
				origin := p.Origin
				if origin == "" && len(p.RelayPath) > 0 {
					// The first name on the path is who originated it; the
					// last is whoever transmitted the copy that was heard.
					origin = named(p.RelayPath[0])
				}
				if origin == "" {
					origin = named(p.Sender)
				}
				if origin == "" {
					unresolved++
					continue
				}
				out = append(out, provider.Reception{
					At: p.At, Receiver: p.Receiver, Origin: origin,
					HopCount: len(p.PathHashes),
					HasSNR:   true, SNRdB: p.SNRdB,
				})
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

	st.Handle("validate.failed", func(w *state.World, p any) (any, error) {
		msg, _ := p.(string)
		w.Jobs = finishJob(w.Jobs, "validate")
		w.Say("validate: " + msg)
		return nil, nil
	})

	// validate.compare: what the model said against what was heard.
	st.Handle("validate.compare", func(w *state.World, p any) (any, error) {
		recs, _ := p.([]provider.Reception)
		w.Jobs = finishJob(w.Jobs, "validate")
		if len(recs) == 0 {
			return nil, fmt.Errorf("no observations in that window")
		}
		obs := observedFrom(recs)
		w.Observed = obs
		res := s.residualsOf(obs, w.Links, w.Nodes)
		w.Residuals = res
		if res == nil || res.Matched == 0 {
			return nil, fmt.Errorf(
				"none of the %d observations matched a pair in this scenario", len(recs))
		}
		// The sign convention, stated: positive means the model predicted a
		// stronger signal than was heard, so the model is optimistic and the
		// excess loss term should go up.
		w.Say(fmt.Sprintf("%d observations matched, median residual %+.1f dB "+
			"(positive means the model is optimistic)", res.Matched, res.MedianDB))
		return map[string]any{
			"matched": res.Matched, "unmatched": res.Unmatched,
			"median_db": res.MedianDB, "iqr_db": res.IQRdB,
			"suggested_excess_loss_db": maxFloat(0, res.MedianDB),
		}, nil
	})

	// validate.calibrate: apply what the comparison found.
	st.Handle("validate.calibrate", func(w *state.World, p any) (any, error) {
		db, have := 0.0, false
		if w.Residuals != nil && w.Residuals.Matched > 0 {
			db, have = maxFloat(0, w.Residuals.MedianDB), true
		}
		if v, ok := numField(p, "db"); ok {
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
		s.excessLossDB, s.excessSet = db, true
		if len(s.nodes) > 0 {
			if err := s.rebuild(w); err != nil {
				return nil, err
			}
			w.Links = nil
			s.warm(st, len(s.nodes))
		}
		w.Say(fmt.Sprintf("excess path loss calibrated to %.1f dB; measuring links", db))
		return map[string]any{"db": db, "links": len(w.Links)}, nil
	})

	// validate.uncalibrate: back to the default, which is a stated guess
	// rather than a measurement.
	st.Handle("validate.uncalibrate", func(w *state.World, _ any) (any, error) {
		s.excessLossDB, s.excessSet = DefaultExcessLossDB, false
		if len(s.nodes) > 0 {
			if err := s.rebuild(w); err != nil {
				return nil, err
			}
			w.Links = nil
			s.warm(st, len(s.nodes))
		}
		w.Say(fmt.Sprintf("excess path loss back to the %d dB default", DefaultExcessLossDB))
		return map[string]any{"db": DefaultExcessLossDB}, nil
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
			HopCount: r.HopCount, HasSNR: r.HasSNR, SNRdB: r.SNRdB,
			PacketID: r.PacketID,
		})
	}
	return out
}
