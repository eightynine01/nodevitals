package agent

import (
	"context"
	"testing"
	"time"

	"github.com/KeiaiLab/nodevitals/internal/collector"
	"github.com/KeiaiLab/nodevitals/internal/config"
	"github.com/KeiaiLab/nodevitals/internal/event"
	"github.com/KeiaiLab/nodevitals/internal/model"
	"github.com/KeiaiLab/nodevitals/internal/sink"
)

type captureSink struct{ events []model.Event }

func (c *captureSink) Name() string { return "capture" }
func (c *captureSink) EmitEvents(_ context.Context, ev []model.Event) error {
	c.events = append(c.events, ev...)
	return nil
}

type fixedCollector struct{ v float64 }

func (f fixedCollector) Name() string { return "fixed" }
func (f fixedCollector) Collect(context.Context) ([]model.Sample, error) {
	return []model.Sample{{Node: "n", Tier: "core", Device: "cpu", Metric: "load1", Value: f.v}}, nil
}

func TestTickCollectsUpdatesMetricsAndDeliversEvents(t *testing.T) {
	cfg := config.Config{
		Node: "n", Tier: "core",
		Rules: []config.Rule{{Metric: "load1", Device: "cpu", Condition: "load_high", Severity: "warning", Threshold: 4, EnterFor: 1, ExitFor: 1}},
	}
	var reg collector.Registry
	reg.Add(fixedCollector{v: 9}) // above threshold → ENTER on first tick (EnterFor=1)
	eng := event.NewEngine("n", cfg.Rules)
	cap := &captureSink{}
	metrics := sink.NewMetrics()

	a := New(cfg, &reg, eng, []sink.Sink{cap}, metrics)
	a.Tick(context.Background())

	if len(cap.events) != 1 || cap.events[0].Phase != model.PhaseEnter {
		t.Fatalf("want 1 ENTER event delivered, got %+v", cap.events)
	}
	if snap := a.Snapshot(); len(snap) != 1 || snap[0].Value != 9 {
		t.Fatalf("snapshot not updated: %+v", snap)
	}
}

func TestTickNoEventWhenBelowThreshold(t *testing.T) {
	cfg := config.Config{Node: "n", Tier: "core",
		Rules: []config.Rule{{Metric: "load1", Device: "cpu", Condition: "load_high", Threshold: 4, EnterFor: 1, ExitFor: 1}}}
	var reg collector.Registry
	reg.Add(fixedCollector{v: 1})
	cap := &captureSink{}
	a := New(cfg, &reg, event.NewEngine("n", cfg.Rules), []sink.Sink{cap}, sink.NewMetrics())
	a.Tick(context.Background())
	if len(cap.events) != 0 {
		t.Fatalf("want no events, got %+v", cap.events)
	}
}

// fakeEventCollector is a Collector that also implements collector.EventSource,
// for exercising Agent.Run's drain path. Collect returns no samples — this
// fake exists only to feed the Events() channel under test control.
type fakeEventCollector struct{ ch chan model.Event }

func (f *fakeEventCollector) Name() string { return "fake-events" }
func (f *fakeEventCollector) Collect(context.Context) ([]model.Sample, error) {
	return nil, nil
}
func (f *fakeEventCollector) Events() <-chan model.Event { return f.ch }

// chanSink is a webhook capture sink backed by a channel instead of a plain
// slice. Run's drain goroutine calls EmitEvents concurrently with the test
// goroutine's assertions, so a plain slice would race; a channel lets the
// test synchronize with select-with-timeout instead of sleep, and is
// race-free by construction.
type chanSink struct{ ch chan model.Event }

func newChanSink() *chanSink { return &chanSink{ch: make(chan model.Event, 4)} }

func (c *chanSink) Name() string { return "chan-capture" }
func (c *chanSink) EmitEvents(_ context.Context, ev []model.Event) error {
	for _, e := range ev {
		c.ch <- e
	}
	return nil
}

func TestRunDrainsEventSourceCollectorsToWebhooksBypassingEngine(t *testing.T) {
	// No Rules: the threshold engine never fires on its own, isolating this
	// test to the drain path (EventSource → webhook, no engine involvement).
	cfg := config.Config{Node: "n", Tier: "gpu"}
	var reg collector.Registry
	// Buffered so the send below never blocks on a reader: synchronization
	// for the actual assertion happens on the receive side (select-with-
	// timeout on cap.ch), not here. An unbuffered channel would hang this
	// test indefinitely — instead of failing fast — if the drain regressed.
	fake := &fakeEventCollector{ch: make(chan model.Event, 1)}
	reg.Add(fake)
	eng := event.NewEngine("n", cfg.Rules)
	cap := newChanSink()

	a := New(cfg, &reg, eng, []sink.Sink{cap}, sink.NewMetrics())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Run(ctx)

	want := model.Event{
		Node: "n", Tier: "gpu", Device: "gpu0",
		Condition: "xid_fatal", Phase: model.PhaseEnter, Severity: model.SevCritical,
		Seq: 1, StartedAt: time.Now().UTC(),
	}
	fake.ch <- want

	select {
	case got := <-cap.ch:
		if got.Condition != want.Condition || got.Device != want.Device || got.Phase != want.Phase {
			t.Fatalf("drained event mismatch: got %+v, want %+v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for drained event to reach webhook sink")
	}

	// Both termination paths — ctx cancel and source channel close — must be
	// safe to trigger without panicking or leaking the drain goroutine.
	cancel()
	close(fake.ch)
}
