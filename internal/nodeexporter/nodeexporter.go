// Package nodeexporter embeds upstream node_exporter's collector set so a
// single nodevitals DaemonSet can serve the full node_* metric surface.
//
// Reimplementing ~50 collectors and 379 metric families would mean re-deriving
// years of kernel-version edge cases, and every one of them would be a place
// for nodevitals to disagree with the ecosystem's de-facto schema. Importing
// the collectors keeps the names, labels, and semantics identical, so existing
// dashboards and alert rules keep working when the separate node_exporter
// DaemonSet is retired.
//
// node_exporter is Apache-2.0; see NOTICE at the repository root.
package nodeexporter

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/node_exporter/collector"
)

// Config selects the host paths the embedded collectors read through. In a
// DaemonSet these are the hostPath mounts, not the container's own /proc.
type Config struct {
	ProcPath     string
	SysPath      string
	RootFSPath   string
	TextfileDir  string
	ExtraFlags   []string
}

// parseOnce guards the kingpin parse. node_exporter's collectors register
// their enable flags on the global kingpin.CommandLine during init(), and stay
// disabled until something parses it — so the embedding process must parse
// exactly once, with the host paths, before building the collector.
var parseOnce sync.Once
var parseErr error

// New returns a prometheus.Collector wrapping every enabled node_exporter
// collector. Register it alongside nodevitals' own metrics to serve both
// surfaces from one endpoint.
func New(cfg Config, log *slog.Logger) (prometheus.Collector, error) {
	args := []string{
		"--path.procfs=" + orDefault(cfg.ProcPath, "/proc"),
		"--path.sysfs=" + orDefault(cfg.SysPath, "/sys"),
		"--path.rootfs=" + orDefault(cfg.RootFSPath, "/"),
	}
	if cfg.TextfileDir != "" {
		args = append(args, "--collector.textfile.directory="+cfg.TextfileDir)
	}
	args = append(args, cfg.ExtraFlags...)

	parseOnce.Do(func() {
		if _, err := kingpin.CommandLine.Parse(args); err != nil {
			parseErr = fmt.Errorf("parse node_exporter flags: %w", err)
		}
	})
	if parseErr != nil {
		return nil, parseErr
	}

	nc, err := collector.NewNodeCollector(log)
	if err != nil {
		return nil, fmt.Errorf("build node_exporter collector: %w", err)
	}
	return nc, nil
}

// Enabled reports the collector names that came back enabled, for a startup
// log line. A silently empty set is the failure mode worth seeing: it means
// the flags never got parsed, and the endpoint would serve no node_* series
// while looking perfectly healthy.
func Enabled(c prometheus.Collector) []string {
	nc, ok := c.(*collector.NodeCollector)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(nc.Collectors))
	for n := range nc.Collectors {
		names = append(names, n)
	}
	return names
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
