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
}
