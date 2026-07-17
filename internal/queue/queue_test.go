package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nodevitals/nodevitals/internal/model"
)

func TestBackoffFullJitterBounded(t *testing.T) {
	b := Backoff{Base: 100 * time.Millisecond, Max: time.Second}
	// attempt 4 → Base*2^4 = 1600ms capped at Max=1s; rnd=1.0 → full window
	if got := b.For(4, 1.0); got != time.Second {
		t.Fatalf("capped delay = %v, want 1s", got)
	}
	// rnd=0 → 0
	if got := b.For(2, 0); got != 0 {
		t.Fatalf("rnd=0 delay = %v, want 0", got)
	}
	// rnd=0.5 attempt0 → Base*1*0.5 = 50ms
	if got := b.For(0, 0.5); got != 50*time.Millisecond {
		t.Fatalf("delay = %v, want 50ms", got)
	}
}

type flakySink struct {
	failFirst int
	calls     int
}

func (f *flakySink) Name() string { return "flaky" }
func (f *flakySink) EmitEvents(context.Context, []model.Event) error {
	f.calls++
	if f.calls <= f.failFirst {
		return errors.New("transient")
	}
	return nil
}

func TestDeliverRetriesThenSucceeds(t *testing.T) {
	s := &flakySink{failFirst: 2}
	var slept []time.Duration
	err := DeliverWithRetry(context.Background(), s, nil, 5,
		Backoff{Base: 10 * time.Millisecond, Max: time.Second},
		func(d time.Duration) { slept = append(slept, d) },
		func() float64 { return 0.0 },
	)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if s.calls != 3 {
		t.Fatalf("want 3 calls, got %d", s.calls)
	}
	if len(slept) != 2 {
		t.Fatalf("want 2 backoff sleeps, got %d", len(slept))
	}
}

func TestDeliverGivesUpAfterMaxAttempts(t *testing.T) {
	s := &flakySink{failFirst: 100}
	err := DeliverWithRetry(context.Background(), s, nil, 3,
		Backoff{Base: time.Millisecond, Max: time.Second},
		func(time.Duration) {}, func() float64 { return 0 },
	)
	if err == nil {
		t.Fatal("expected failure after max attempts")
	}
	if s.calls != 3 {
		t.Fatalf("want 3 attempts, got %d", s.calls)
	}
}
