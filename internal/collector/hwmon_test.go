package collector

import (
	"context"
	"strings"
	"testing"

	"github.com/KeiaiLab/nodevitals/internal/model"
)

func TestHwmonReadsTempFixture(t *testing.T) {
	c := NewHwmon("n", "../../testdata/sys")
	got, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	for i := range got {
		if got[i].Kind != model.KindGauge {
			t.Fatalf("hwmon metrics must be gauges, got kind %q", got[i].Kind)
		}
	}
	var temp *float64
	for i := range got {
		if got[i].Metric == "temp_celsius" {
			temp = &got[i].Value
		}
	}
	if temp == nil {
		t.Fatalf("no temp_celsius sample: %+v", got)
	}
	if *temp != 45.0 {
		t.Fatalf("temp = %v, want 45.0", *temp)
	}
}

func TestHwmonMissingDirIsEmptyNotError(t *testing.T) {
	c := NewHwmon("n", "/nonexistent")
	got, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("missing hwmon dir should be empty, not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 samples, got %d", len(got))
	}
}

func TestHwmonDistinctDevicesForSameDriverName(t *testing.T) {
	c := NewHwmon("n", "../../testdata/sys-multi")
	got, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	temps := map[string]float64{}
	for _, s := range got {
		if s.Metric == "temp_celsius" {
			temps[s.Device] = s.Value
		}
	}
	if len(temps) != 2 {
		t.Fatalf("two nvme chips must produce two distinct Devices, got %d: %+v", len(temps), temps)
	}
	// both Devices must reference the nvme driver name and be distinct
	for dev := range temps {
		if !strings.Contains(dev, "nvme/temp1") {
			t.Fatalf("device %q should contain nvme/temp1", dev)
		}
	}
}
