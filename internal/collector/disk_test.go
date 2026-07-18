package collector

import (
	"context"
	"testing"
)

func TestDiskReadsFixture(t *testing.T) {
	c := NewDisk("n", "../../testdata/proc", "../../testdata/sys")
	got, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m := map[string]float64{}
	for _, s := range got {
		if s.Device == "sda" {
			m[s.Metric] = s.Value
		}
	}
	if m["disk_read_bytes"] != 2000*512 {
		t.Fatalf("read_bytes = %v, want %v", m["disk_read_bytes"], 2000*512)
	}
	if m["disk_write_ios"] != 50 {
		t.Fatalf("write_ios = %v, want 50", m["disk_write_ios"])
	}
}
