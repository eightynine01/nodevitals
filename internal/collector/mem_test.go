package collector

import (
	"context"
	"testing"
)

func TestMemReadsFixture(t *testing.T) {
	c := NewMem("n", "../../testdata/proc")
	got, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := map[string]float64{}
	for _, s := range got {
		if s.Device != "mem" {
			t.Fatalf("device = %q, want mem", s.Device)
		}
		m[s.Metric] = s.Value
	}
	if m["mem_total_bytes"] != 16000000*1024 {
		t.Fatalf("total = %v", m["mem_total_bytes"])
	}
	if m["mem_available_bytes"] != 8000000*1024 {
		t.Fatalf("available = %v", m["mem_available_bytes"])
	}
	// used = total - available = 8,000,000 kB
	if m["mem_used_bytes"] != 8000000*1024 {
		t.Fatalf("used = %v", m["mem_used_bytes"])
	}
	// swap_used = swap_total - swap_free = (4,000,000 - 3,000,000) * 1024 = 1,000,000 * 1024
	if m["swap_used_bytes"] != 1000000*1024 {
		t.Fatalf("swap_used = %v, want %v", m["swap_used_bytes"], 1000000*1024)
	}
}

func TestMemMissingSwapNoPanicOmitsSwapMetric(t *testing.T) {
	c := NewMem("n", "../../testdata/proc-noswap")
	got, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := map[string]float64{}
	for _, s := range got {
		m[s.Metric] = s.Value
	}
	// swap fields absent → nil-guard must skip swap_used_bytes (no panic, no key)
	if _, ok := m["swap_used_bytes"]; ok {
		t.Fatalf("swap_used_bytes must be omitted when swap fields absent, got %v", m["swap_used_bytes"])
	}
	// memory fields must still be present
	if m["mem_total_bytes"] != 16000000*1024 {
		t.Fatalf("mem_total_bytes = %v, want %v", m["mem_total_bytes"], 16000000*1024)
	}
}
