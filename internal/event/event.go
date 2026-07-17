// Package event turns a stream of samples into hardware state-transition events.
// It is deterministic and hardware-free: give it samples, get events.
package event

import (
	"time"

	"github.com/nodevitals/nodevitals/internal/config"
	"github.com/nodevitals/nodevitals/internal/model"
)

type ruleState struct {
	rule       config.Rule
	active     bool
	breachRun  int
	clearRun   int
	seq        uint64
	startedAt  time.Time
}

// Engine evaluates rules against samples, holding per-rule hysteresis state.
type Engine struct {
	node  string
	rules map[string]*ruleState // key: condition
}

func NewEngine(node string, rules []config.Rule) *Engine {
	m := make(map[string]*ruleState, len(rules))
	for _, r := range rules {
		m[r.Condition] = &ruleState{rule: r}
	}
	return &Engine{node: node, rules: m}
}

// Evaluate returns any state-transition events triggered by these samples.
func (e *Engine) Evaluate(samples []model.Sample) []model.Event {
	var out []model.Event
	now := time.Now().UTC()
	for _, s := range samples {
		for _, st := range e.rules {
			if st.rule.Metric != s.Metric || st.rule.Device != s.Device {
				continue
			}
			breached := s.Value > st.rule.Threshold
			if breached {
				st.breachRun++
				st.clearRun = 0
			} else {
				st.clearRun++
				st.breachRun = 0
			}

			enterFor := max1(st.rule.EnterFor)
			exitFor := max1(st.rule.ExitFor)

			if !st.active && st.breachRun >= enterFor {
				st.active = true
				st.seq++
				st.startedAt = now
				out = append(out, model.Event{
					Node: e.node, Tier: s.Tier, Device: s.Device,
					Condition: st.rule.Condition, Phase: model.PhaseEnter,
					Severity: st.rule.Severity, Seq: st.seq, StartedAt: now,
					Detail: map[string]any{"value": s.Value, "threshold": st.rule.Threshold},
				})
			} else if st.active && st.clearRun >= exitFor {
				st.active = false
				st.seq++
				out = append(out, model.Event{
					Node: e.node, Tier: s.Tier, Device: s.Device,
					Condition: st.rule.Condition, Phase: model.PhaseExit,
					Severity: st.rule.Severity, Seq: st.seq,
					StartedAt: st.startedAt, EndedAt: now,
					Detail: map[string]any{"value": s.Value, "threshold": st.rule.Threshold},
				})
			}
		}
	}
	for i := range out {
		out[i].ID = out[i].Fingerprint()
	}
	return out
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
