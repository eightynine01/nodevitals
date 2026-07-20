package sink

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KeiaiLab/nodevitals/internal/model"
)

func TestMetricsExposesLatestSample(t *testing.T) {
	m := NewMetrics()
	m.Update([]model.Sample{
		{Node: "n1", Tier: "core", Device: "cpu", Metric: "load1", Value: 1.5},
	})

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	if !strings.Contains(body, `nodevitals_hw_load1`) {
		t.Fatalf("missing metric name in output:\n%s", body)
	}
	if !strings.Contains(body, `device="cpu"`) || !strings.Contains(body, `1.5`) {
		t.Fatalf("missing labels/value:\n%s", body)
	}
}

func TestMetricsExposesCounterWithTotalNameAndType(t *testing.T) {
	m := NewMetrics()
	m.Update([]model.Sample{
		{Node: "n1", Tier: "core", Device: "eth0", Metric: "net_rx_bytes_total", Kind: model.KindCounter, Value: 5000},
	})
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)
	if !strings.Contains(body, "# TYPE nodevitals_hw_net_rx_bytes_total counter") {
		t.Fatalf("counter TYPE declaration missing:\n%s", body)
	}
	if !strings.Contains(body, `nodevitals_hw_net_rx_bytes_total{device="eth0"`) {
		t.Fatalf("counter sample line missing:\n%s", body)
	}
}

func TestMetricsUpdateReplacesSnapshot(t *testing.T) {
	m := NewMetrics()
	m.Update([]model.Sample{{Node: "n", Tier: "core", Device: "cpu", Metric: "load1", Value: 1}})
	m.Update([]model.Sample{{Node: "n", Tier: "core", Device: "cpu", Metric: "load1", Value: 9}})

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), "9") {
		t.Fatalf("expected updated value 9:\n%s", string(raw))
	}
}

// TestMetricsPromotesSampleLabels is the load-bearing proof that G1 identity
// labels reach /metrics: the sink must promote Sample.Labels onto the const
// metric's variable labels. A 200 (not 500) also proves the promoted desc is
// self-consistent — this test fails before the label-promotion change.
func TestMetricsPromotesSampleLabels(t *testing.T) {
	m := NewMetrics()
	m.Update([]model.Sample{{
		Node: "n1", Tier: "gpu", Device: "gpu0", Metric: "gpu_utilization_pct", Value: 55,
		Labels: map[string]string{
			"gpu_uuid":   "GPU-abc",
			"gpu_model":  "NVIDIA A100",
			"pci_bus_id": "00000000:65:00.0",
		},
	}})

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (inconsistent descriptor would 500):\n%s", resp.StatusCode, body)
	}

	var line string
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, "nodevitals_hw_gpu_utilization_pct{") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("no nodevitals_hw_gpu_utilization_pct sample line:\n%s", body)
	}
	for _, want := range []string{
		`gpu_uuid="GPU-abc"`,
		`gpu_model="NVIDIA A100"`,
		`pci_bus_id="00000000:65:00.0"`,
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("utilization line missing promoted label %s:\n%s", want, line)
		}
	}
}

// TestMetricsMixedLabeledAndUnlabeledStaysConsistent guards backward compat and
// determinism: a labeled gpu sample and a nil-Labels core sample in the same
// snapshot must both scrape 200, with the unlabeled one keeping the bare
// [node,tier,device] shape (no collision, no inconsistent-descriptor 500).
func TestMetricsMixedLabeledAndUnlabeledStaysConsistent(t *testing.T) {
	m := NewMetrics()
	m.Update([]model.Sample{
		{Node: "n1", Tier: "gpu", Device: "gpu0", Metric: "gpu_utilization_pct", Value: 55,
			Labels: map[string]string{"gpu_uuid": "GPU-abc"}},
		{Node: "n1", Tier: "core", Device: "cpu", Metric: "load1", Value: 1.5}, // nil Labels
	})

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200:\n%s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "nodevitals_hw_gpu_utilization_pct") {
		t.Fatalf("labeled gpu metric missing:\n%s", body)
	}
	// unlabeled sample keeps the exact bare shape (labels sorted alphabetically).
	if !strings.Contains(body, `nodevitals_hw_load1{device="cpu",node="n1",tier="core"} 1.5`) {
		t.Fatalf("unlabeled metric should keep bare [node,tier,device] shape:\n%s", body)
	}
}

func TestMetricsRecordDroppedExposesCounter(t *testing.T) {
	m := NewMetrics()
	m.RecordDropped("webhook-a", 3)
	m.RecordDropped("webhook-a", 2) // accumulates → 5
	m.RecordDropped("webhook-b", 1)
	m.RecordDropped("webhook-b", 0) // no-op

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	if !strings.Contains(body, "# TYPE nodevitals_delivery_dropped_total counter") {
		t.Fatalf("dropped counter TYPE missing:\n%s", body)
	}
	if !strings.Contains(body, `nodevitals_delivery_dropped_total{sink="webhook-a"} 5`) {
		t.Fatalf("want webhook-a dropped=5:\n%s", body)
	}
	if !strings.Contains(body, `nodevitals_delivery_dropped_total{sink="webhook-b"} 1`) {
		t.Fatalf("want webhook-b dropped=1:\n%s", body)
	}
}
