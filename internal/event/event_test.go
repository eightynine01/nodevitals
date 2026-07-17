package event

import (
	"testing"
	"time"

	"github.com/nodevitals/nodevitals/internal/config"
	"github.com/nodevitals/nodevitals/internal/model"
)

func rule() config.Rule {
	return config.Rule{
		Metric: "load1", Device: "cpu", Condition: "load_high",
		Severity: "warning", Threshold: 4.0, EnterFor: 2, ExitFor: 2,
	}
}

func sample(v float64) []model.Sample {
	return []model.Sample{{Node: "n", Tier: "core", Device: "cpu", Metric: "load1", Value: v, Timestamp: time.Now()}}
}

func TestEnterFiresAfterEnterForConsecutiveBreaches(t *testing.T) {
	e := NewEngine("n", []config.Rule{rule()})
	if ev := e.Evaluate(sample(5)); len(ev) != 0 {
		t.Fatalf("breach 1/2 must not fire, got %d", len(ev))
	}
	ev := e.Evaluate(sample(5))
	if len(ev) != 1 || ev[0].Phase != model.PhaseEnter {
		t.Fatalf("breach 2/2 must ENTER, got %+v", ev)
	}
	if ev[0].Condition != "load_high" || ev[0].Severity != "warning" || ev[0].Seq != 1 {
		t.Fatalf("bad enter event: %+v", ev[0])
	}
}

func TestNoDuplicateEnterWhileActive(t *testing.T) {
	e := NewEngine("n", []config.Rule{rule()})
	e.Evaluate(sample(5))
	e.Evaluate(sample(5)) // ENTER
	if ev := e.Evaluate(sample(6)); len(ev) != 0 {
		t.Fatalf("must not re-ENTER while active, got %+v", ev)
	}
}

func TestExitFiresAfterExitForConsecutiveClears(t *testing.T) {
	e := NewEngine("n", []config.Rule{rule()})
	e.Evaluate(sample(5))
	e.Evaluate(sample(5)) // ENTER (seq 1)
	if ev := e.Evaluate(sample(1)); len(ev) != 0 {
		t.Fatalf("clear 1/2 must not EXIT, got %+v", ev)
	}
	ev := e.Evaluate(sample(1))
	if len(ev) != 1 || ev[0].Phase != model.PhaseExit {
		t.Fatalf("clear 2/2 must EXIT, got %+v", ev)
	}
	if ev[0].Seq != 2 || ev[0].EndedAt.IsZero() {
		t.Fatalf("exit must carry seq=2 and EndedAt: %+v", ev[0])
	}
}

func TestHysteresisResetsClearCountOnRebreapch(t *testing.T) {
	e := NewEngine("n", []config.Rule{rule()})
	e.Evaluate(sample(5))
	e.Evaluate(sample(5)) // ENTER
	e.Evaluate(sample(1)) // clear 1/2
	e.Evaluate(sample(9)) // breach again → clear count resets
	if ev := e.Evaluate(sample(1)); len(ev) != 0 {
		t.Fatalf("clear count must have reset; single clear must not EXIT, got %+v", ev)
	}
}
