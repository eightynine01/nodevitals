package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/KeiaiLab/nodevitals/internal/model"
	"github.com/prometheus/procfs"
)

type netCollector struct {
	node     string
	procRoot string
}

// NewNet reports per-interface network counters from /proc/net/dev (loopback skipped).
func NewNet(node, procRoot string) Collector { return &netCollector{node: node, procRoot: procRoot} }

func (c *netCollector) Name() string { return "net" }

func (c *netCollector) Collect(ctx context.Context) ([]model.Sample, error) {
	fs, err := procfs.NewFS(c.procRoot)
	if err != nil {
		return nil, fmt.Errorf("open procfs %q: %w", c.procRoot, err)
	}
	nd, err := fs.NetDev()
	if err != nil {
		return nil, fmt.Errorf("read net/dev: %w", err)
	}
	now := time.Now().UTC()
	var out []model.Sample
	for iface, line := range nd {
		if iface == "lo" {
			continue
		}
		mk := func(metric string, v uint64) model.Sample {
			return model.Sample{Node: c.node, Tier: "core", Device: iface, Metric: metric, Kind: model.KindCounter, Value: float64(v), Timestamp: now}
		}
		out = append(out,
			mk("net_rx_bytes_total", line.RxBytes), mk("net_tx_bytes_total", line.TxBytes),
			mk("net_rx_errors_total", line.RxErrors), mk("net_tx_errors_total", line.TxErrors))
	}
	return out, nil
}
