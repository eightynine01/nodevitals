package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/procfs"

	"github.com/KeiaiLab/nodevitals/internal/model"
)

type loadAvg struct {
	node     string
	procRoot string
}

// NewLoadAvg reads 1-minute load average from <procRoot>/loadavg.
func NewLoadAvg(node, procRoot string) Collector {
	return &loadAvg{node: node, procRoot: procRoot}
}

func (l *loadAvg) Name() string { return "loadavg" }

func (l *loadAvg) Collect(ctx context.Context) ([]model.Sample, error) {
	fs, err := procfs.NewFS(l.procRoot)
	if err != nil {
		return nil, fmt.Errorf("open procfs %q: %w", l.procRoot, err)
	}
	la, err := fs.LoadAvg()
	if err != nil {
		return nil, fmt.Errorf("read loadavg: %w", err)
	}
	return []model.Sample{{
		Node: l.node, Tier: "core", Device: "cpu", Metric: "load1",
		Value: la.Load1, Timestamp: time.Now().UTC(),
	}}, nil
}
