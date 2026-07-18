package collector

import (
	"context"
	"testing"

	"github.com/KeiaiLab/nodevitals/internal/model"
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
			if s.Kind != model.KindCounter {
				t.Fatalf("sda sample %s must be KindCounter, got %q", s.Metric, s.Kind)
			}
		}
	}
	if m["disk_read_bytes_total"] != 2000*512 {
		t.Fatalf("read_bytes_total = %v, want %v", m["disk_read_bytes_total"], 2000*512)
	}
	if m["disk_write_ios_total"] != 50 {
		t.Fatalf("write_ios_total = %v, want 50", m["disk_write_ios_total"])
	}
}
