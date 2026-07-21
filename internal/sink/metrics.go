package sink

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"

	"github.com/KeiaiLab/nodevitals/internal/model"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics exposes the latest sample snapshot as Prometheus gauges. It implements
// prometheus.Collector, emitting const metrics on scrape from the held snapshot.
type Metrics struct {
	mu       sync.RWMutex
	snapshot []model.Sample
	reg      *prometheus.Registry
	dropped  *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	m := &Metrics{
		reg: prometheus.NewRegistry(),
		dropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nodevitals_delivery_dropped_total",
			Help: "events dropped after exhausting webhook delivery retries (silent-loss signal)",
		}, []string{"sink"}),
	}
	m.reg.MustRegister(m, m.dropped)
	return m
}

// Register adds an extra collector to the exposed registry — used to serve the
// embedded node_exporter surface from the same /metrics endpoint.
func (m *Metrics) Register(c prometheus.Collector) error {
	return m.reg.Register(c)
}

// RecordDropped increments the drop counter for a sink by n events. Called when
// DeliverWithRetry exhausts its retries and the batch is lost, so operators can
// alert on otherwise-silent delivery loss.
func (m *Metrics) RecordDropped(sink string, n int) {
	if n > 0 {
		m.dropped.WithLabelValues(sink).Add(float64(n))
	}
}

// Update replaces the exposed snapshot atomically.
func (m *Metrics) Update(samples []model.Sample) {
	m.mu.Lock()
	m.snapshot = samples
	m.mu.Unlock()
}

func (m *Metrics) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(m, ch)
}

func (m *Metrics) Collect(ch chan<- prometheus.Metric) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.snapshot {
		vt := prometheus.GaugeValue
		if s.Kind == model.KindCounter {
			vt = prometheus.CounterValue
		}
		// Promote Sample.Labels onto the const metric's variable labels after
		// the fixed [node,tier,device]. sort.Strings is mandatory: Go map order
		// is randomized, so unsorted keys would give the same metric name descs
		// with different label ORDER across samples/scrapes, which registry.Gather
		// rejects as inconsistent descriptors → /metrics 500. Nil/empty Labels
		// yield exactly [node,tier,device] as before (backward compatible).
		labelNames := []string{"node", "tier", "device"}
		labelValues := []string{s.Node, s.Tier, s.Device}
		keys := make([]string, 0, len(s.Labels))
		for k := range s.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			labelNames = append(labelNames, k)
			labelValues = append(labelValues, s.Labels[k])
		}
		desc := prometheus.NewDesc(
			"nodevitals_hw_"+s.Metric,
			"nodevitals hardware metric "+s.Metric,
			labelNames, nil,
		)
		ch <- prometheus.MustNewConstMetric(desc, vt, s.Value, labelValues...)
	}
}

// Handler returns the /metrics HTTP handler.
func (m *Metrics) Handler() http.Handler {
	// ContinueOnError, not the default HTTPErrorOnError.
	//
	// A textfile collector is user-supplied, so it can export a metric name a
	// built-in collector also owns — with different help text. client_golang
	// treats that as fatal and rejects the WHOLE scrape with a 500, so one
	// stray .prom file silently takes out every series on that node. That is
	// exactly what happened here: an ansible-written edac_metrics.prom
	// collided with the built-in edac collector, and only on the nodes whose
	// hardware actually registers a memory controller — two nodes served zero
	// metrics while looking healthy on /healthz.
	//
	// Upstream node_exporter makes the same choice for the same reason. The
	// error still surfaces: it goes to the log, and the affected collector's
	// node_scrape_collector_success drops to 0.
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{
		Registry:      m.reg,
		ErrorHandling: promhttp.ContinueOnError,
		ErrorLog:      slogErrorLog{},
	})
}

// slogErrorLog adapts promhttp's logger to slog so scrape-time collector
// errors land in the agent's normal log stream instead of vanishing.
type slogErrorLog struct{}

func (slogErrorLog) Println(v ...any) {
	slog.Warn("metrics scrape", "err", fmt.Sprint(v...))
}
