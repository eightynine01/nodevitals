# nodevitals M2a — Core Collectors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Complete the core tier — migrate loadavg to `prometheus/procfs` and add cpu-util, mem, net, disk, and hwmon collectors, all hardware-free tested and wired into the running agent + Helm chart.

**Architecture:** Reuse M1's `collector.Collector` interface (`Collect(ctx) ([]model.Sample, error)`) and `Registry`. New collectors read `/proc` via `procfs.NewFS(procRoot)` (root-injectable → fixture-testable), except hwmon which hand-parses `<sysRoot>/class/hwmon` (procfs has no hwmon support). Stays on the M1 `CGO_ENABLED=0` static image — no cgo.

**Tech Stack:** Go 1.26 · `github.com/prometheus/procfs` (+ `.../blockdevice` for diskstats) · stdlib for hwmon.

## Global Constraints

- **Module path**: `github.com/KeiaiLab/nodevitals`. **Go**: 1.26 (`go.mod` go-directive stays `1.26`).
- **New dependency**: `github.com/prometheus/procfs` (Apache-2.0) only. No cgo — core tier stays static. Run `go mod tidy` after adding (avoid the `// indirect` mislabel — see M1 Task 3/7 history).
- **Root injection MUST persist**: every collector takes an injected filesystem root (`procRoot` for /proc collectors, `sysRoot` for hwmon) — NO hardcoded `/proc` or `/sys`. This is what keeps tests hardware-free on macOS/CI. procfs supports this via `procfs.NewFS(procRoot)`.
- **Collector interface unchanged**: `collector.NewX(node string, root string) collector.Collector` returning something whose `Collect(ctx)` yields `[]model.Sample` with `Tier:"core"`. Follow the M1 loadavg collector shape exactly.
- **Sample identity**: `Node` from arg, `Tier:"core"`, `Device` per collector (`cpu`, `mem`, per-interface name, per-disk name, per-sensor chip), `Metric` snake_case, `Timestamp: time.Now().UTC()`.
- **Test hygiene**: fixture files under `testdata/proc/*` and `testdata/sys/*`; deterministic; pristine output; `sleep` forbidden. Each collector's error path (missing/malformed file) returns a wrapped error.
- **Commits**: Conventional Commits, one per task, commit locally (push to origin/main is the controller's call at the end).

---

## File Structure (M2a end state — additions to the existing repo)

```
internal/collector/
  loadavg.go  loadavg_test.go        # MIGRATE to procfs.NewFS
  cpu.go      cpu_test.go            # NEW — stateful delta
  mem.go      mem_test.go            # NEW
  net.go      net_test.go            # NEW
  disk.go     disk_test.go           # NEW (blockdevice)
  hwmon.go    hwmon_test.go          # NEW (hand-rolled sysfs)
internal/config/config.go            # MODIFY: add SysRoot field
cmd/nodevitals/main.go               # MODIFY: register the 5 new collectors
deploy/chart/templates/daemonset.yaml # MODIFY: mount /sys (hwmon) read-only
deploy/chart/values.yaml             # MODIFY: default rules for new metrics
testdata/proc/{stat,meminfo,net/dev} # NEW fixtures
testdata/proc/diskstats              # NEW fixture
testdata/sys/class/hwmon/...         # NEW fixtures
```

**Order**: Task 1 (procfs dep + loadavg migration) → 2 cpu → 3 mem → 4 net → 5 disk → 6 hwmon → 7 wire main.go → 8 chart+config+smoke.

> **procfs API note for implementers** — verify exact field names against `go doc github.com/prometheus/procfs` after `go get` (the API is stable but confirm): `procfs.NewFS(root) (FS, error)`; `FS.LoadAvg() (*LoadAvg{Load1,Load5,Load15 float64}, error)`; `FS.Stat() (Stat, error)` where `Stat.CPUTotal CPUStat` (aggregate) and `Stat.CPU map[int64]CPUStat`, `CPUStat{User,Nice,System,Idle,Iowait,IRQ,SoftIRQ,Steal,Guest,GuestNice float64}` (seconds); `FS.Meminfo() (Meminfo, error)` with `*uint64` fields incl. `MemTotalBytes,MemFreeBytes,MemAvailableBytes,SwapTotalBytes,SwapFreeBytes` (nil if absent); `FS.NetDev() (NetDev=map[string]NetDevLine, error)`, `NetDevLine{Name string; RxBytes,RxErrors,TxBytes,TxErrors uint64; ...}`; `blockdevice.NewFS(procRoot,sysRoot) (FS,error)`, `FS.ProcDiskstats() ([]Diskstats,error)`, `Diskstats{DeviceName string; IOStats{ReadIOs,ReadSectors,WriteIOs,WriteSectors uint64; ...}}`.

---

### Task 1: Add procfs + migrate loadavg to procfs.NewFS

**Files:** Modify `internal/collector/loadavg.go`, `internal/collector/loadavg_test.go`; add `go.mod`/`go.sum`. Keep `testdata/proc/loadavg`.

**Interfaces:**
- Produces: `collector.NewLoadAvg(node, procRoot string) Collector` (signature unchanged) now backed by procfs.

- [ ] **Step 1: add dep**
```bash
go get github.com/prometheus/procfs
```

- [ ] **Step 2: verify existing loadavg test still expresses intent** — the M1 test `TestLoadAvgReadsFixture` reads `testdata/proc/loadavg` (fixture `0.52 0.58 0.59 2/1234 56789`) and asserts `load1==0.52`. Keep it as-is; it must still pass after migration.

- [ ] **Step 3: migrate implementation** — replace the hand-rolled body of `internal/collector/loadavg.go`'s `Collect` with procfs:
```go
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

func NewLoadAvg(node, procRoot string) Collector { return &loadAvg{node: node, procRoot: procRoot} }

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
```

- [ ] **Step 4: run + tidy**
```bash
go test ./internal/collector/ -run TestLoadAvg -v   # expect PASS (2 tests)
go mod tidy                                          # ensure procfs is a DIRECT require (no // indirect)
grep 'prometheus/procfs' go.mod                      # must NOT show // indirect
grep '^go ' go.mod                                   # must be go 1.26
go build ./... && go vet ./...
```
Expected: loadavg tests PASS, procfs direct dep, go 1.26, build+vet clean.

- [ ] **Step 5: commit**
```bash
git commit -am "refactor(collector): back loadavg with prometheus/procfs (root-injectable)"
```

---

### Task 2: CPU utilization collector (stateful delta)

CPU util needs two samples to compute a delta, so this collector holds the previous `/proc/stat` snapshot. First tick establishes a baseline and emits nothing.

**Files:** Create `internal/collector/cpu.go`, `internal/collector/cpu_test.go`, `testdata/proc/stat`.

**Interfaces:**
- Produces: `collector.NewCPU(node, procRoot string) Collector`. Emits `Metric:"cpu_util_pct"`, `Device:"cpu"` (aggregate) after the 2nd+ Collect; per-cpu (`Device:"cpu0"...`) too.

- [ ] **Step 1: fixture** — Create `testdata/proc/stat` (two-line minimal — total + cpu0; jiffies):
```
cpu  100 0 100 800 0 0 0 0 0 0
cpu0 100 0 100 800 0 0 0 0 0 0
```
And a second fixture `testdata/proc/stat2` representing a later reading (more busy time):
```
cpu  200 0 200 1200 0 0 0 0 0 0
cpu0 200 0 200 1200 0 0 0 0 0 0
```

- [ ] **Step 2: failing test** — Create `internal/collector/cpu_test.go`:
```go
package collector

import (
	"context"
	"testing"
)

func TestCPUFirstTickIsBaselineNoSamples(t *testing.T) {
	c := NewCPU("n", "../../testdata/proc") // reads stat (busy=200 of 1000)
	got, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("first tick must be baseline (no delta yet), got %d samples", len(got))
	}
}

func TestCPUUtilComputedFromDelta(t *testing.T) {
	c := &cpuCollector{node: "n", procRoot: "../../testdata/proc", statFile: "stat"}
	if _, err := c.Collect(context.Background()); err != nil { // baseline from stat
		t.Fatalf("baseline: %v", err)
	}
	c.statFile = "stat2" // next reading: total busy 200+200=400 of 400+1200=1600; delta busy=300 idle=400 → 300/700≈42.86%
	got, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var total *float64
	for i := range got {
		if got[i].Device == "cpu" && got[i].Metric == "cpu_util_pct" {
			total = &got[i].Value
		}
	}
	if total == nil {
		t.Fatalf("no aggregate cpu_util_pct sample: %+v", got)
	}
	if *total < 42.0 || *total > 43.0 {
		t.Fatalf("cpu_util_pct = %.2f, want ~42.86", *total)
	}
}
```

- [ ] **Step 3: run — expect FAIL** (`NewCPU`/`cpuCollector` undefined). `go test ./internal/collector/ -run TestCPU -v`

- [ ] **Step 4: implement** — Create `internal/collector/cpu.go`. `statFile` is an injectable filename (default `"stat"`) so the test can swap the second reading; production always uses `"stat"` and relies on real time passing between ticks:
```go
package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/procfs"
	"github.com/KeiaiLab/nodevitals/internal/model"
)

type cpuCollector struct {
	node     string
	procRoot string
	statFile string // test seam; "" → procfs default ("stat")
	prev     map[string]procfs.CPUStat
}

// NewCPU reports per-CPU and aggregate utilization percentage from /proc/stat deltas.
func NewCPU(node, procRoot string) Collector {
	return &cpuCollector{node: node, procRoot: procRoot, prev: map[string]procfs.CPUStat{}}
}

func (c *cpuCollector) Name() string { return "cpu" }

func busyIdle(s procfs.CPUStat) (busy, idle float64) {
	idle = s.Idle + s.Iowait
	busy = s.User + s.Nice + s.System + s.IRQ + s.SoftIRQ + s.Steal
	return
}

func (c *cpuCollector) Collect(ctx context.Context) ([]model.Sample, error) {
	fs, err := procfs.NewFS(c.procRoot)
	if err != nil {
		return nil, fmt.Errorf("open procfs %q: %w", c.procRoot, err)
	}
	stat, err := fs.Stat()
	if err != nil {
		return nil, fmt.Errorf("read stat: %w", err)
	}
	cur := map[string]procfs.CPUStat{"cpu": stat.CPUTotal}
	for id, cs := range stat.CPU {
		cur[fmt.Sprintf("cpu%d", id)] = cs
	}
	now := time.Now().UTC()
	var out []model.Sample
	if c.prev != nil {
		for dev, s := range cur {
			p, ok := c.prev[dev]
			if !ok {
				continue
			}
			bNow, iNow := busyIdle(s)
			bPrev, iPrev := busyIdle(p)
			db, di := bNow-bPrev, iNow-iPrev
			denom := db + di
			if denom <= 0 {
				continue
			}
			out = append(out, model.Sample{
				Node: c.node, Tier: "core", Device: dev, Metric: "cpu_util_pct",
				Value: 100 * db / denom, Timestamp: now,
			})
		}
	}
	c.prev = cur
	return out, nil
}
```
Note: if `c.statFile` is set (test), read that file instead — implement by opening `procfs.NewFS` at a temp dir isn't possible per-file, so instead when `statFile != ""` read `<procRoot>/<statFile>` and parse via `procfs`'s parser is not exposed. SIMPLER: for the test seam, have the test point `procRoot` at a dir whose `stat` is the desired reading. **Implementer: drop the `statFile` field and instead make `TestCPUUtilComputedFromDelta` use two fixture directories** (`testdata/proc` with `stat`, and `testdata/proc2` with the busier `stat`), calling Collect once against each via two `procRoot` values on the SAME collector instance (set `c.procRoot` between calls). Adjust the test accordingly; keep the baseline+delta assertions. This keeps everything going through procfs.

- [ ] **Step 5: adjust fixtures/test per the note** — create `testdata/proc2/stat` (the busier reading), and in `TestCPUUtilComputedFromDelta` set `c := &cpuCollector{node:"n", procRoot:"../../testdata/proc", prev: map[string]procfs.CPUStat{}}`, Collect (baseline), then `c.procRoot = "../../testdata/proc2"`, Collect (delta), assert ~42.86.

- [ ] **Step 6: run — expect PASS.** `go test ./internal/collector/ -run TestCPU -v`

- [ ] **Step 7: commit** `git commit -am "feat(collector): cpu utilization from /proc/stat deltas"`

---

### Task 3: Memory collector

**Files:** Create `internal/collector/mem.go`, `mem_test.go`, `testdata/proc/meminfo`.

**Interfaces:** `collector.NewMem(node, procRoot string) Collector`. Emits `Device:"mem"`, metrics `mem_used_bytes`, `mem_available_bytes`, `mem_total_bytes`, `swap_used_bytes`.

- [ ] **Step 1: fixture** `testdata/proc/meminfo`:
```
MemTotal:       16000000 kB
MemFree:         2000000 kB
MemAvailable:    8000000 kB
SwapTotal:       4000000 kB
SwapFree:        3000000 kB
```

- [ ] **Step 2: failing test** `internal/collector/mem_test.go`:
```go
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
```

- [ ] **Step 3: run — FAIL.** `go test ./internal/collector/ -run TestMem -v`

- [ ] **Step 4: implement** `internal/collector/mem.go`:
```go
package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/procfs"
	"github.com/KeiaiLab/nodevitals/internal/model"
)

type memCollector struct {
	node     string
	procRoot string
}

// NewMem reports memory and swap usage from /proc/meminfo.
func NewMem(node, procRoot string) Collector { return &memCollector{node: node, procRoot: procRoot} }

func (c *memCollector) Name() string { return "mem" }

func (c *memCollector) Collect(ctx context.Context) ([]model.Sample, error) {
	fs, err := procfs.NewFS(c.procRoot)
	if err != nil {
		return nil, fmt.Errorf("open procfs %q: %w", c.procRoot, err)
	}
	mi, err := fs.Meminfo()
	if err != nil {
		return nil, fmt.Errorf("read meminfo: %w", err)
	}
	now := time.Now().UTC()
	s := func(metric string, v float64) model.Sample {
		return model.Sample{Node: c.node, Tier: "core", Device: "mem", Metric: metric, Value: v, Timestamp: now}
	}
	var out []model.Sample
	// Meminfo *Bytes fields are already byte-normalized; nil-guard each.
	if mi.MemTotalBytes != nil {
		out = append(out, s("mem_total_bytes", float64(*mi.MemTotalBytes)))
	}
	if mi.MemAvailableBytes != nil {
		out = append(out, s("mem_available_bytes", float64(*mi.MemAvailableBytes)))
		if mi.MemTotalBytes != nil {
			out = append(out, s("mem_used_bytes", float64(*mi.MemTotalBytes-*mi.MemAvailableBytes)))
		}
	}
	if mi.SwapTotalBytes != nil && mi.SwapFreeBytes != nil {
		out = append(out, s("swap_used_bytes", float64(*mi.SwapTotalBytes-*mi.SwapFreeBytes)))
	}
	return out, nil
}
```
(Implementer: confirm `MemTotalBytes` etc. exist on `procfs.Meminfo`; if the installed version only exposes the KB `*uint64` fields — `MemTotal` etc. — multiply by 1024 instead.)

- [ ] **Step 5: run — PASS.** `go test ./internal/collector/ -run TestMem -v`
- [ ] **Step 6: commit** `git commit -am "feat(collector): memory/swap from /proc/meminfo"`

---

### Task 4: Network collector

**Files:** Create `internal/collector/net.go`, `net_test.go`, `testdata/proc/net/dev`.

**Interfaces:** `collector.NewNet(node, procRoot string) Collector`. Per-interface `Device:"<ifname>"`, metrics `net_rx_bytes`, `net_tx_bytes`, `net_rx_errors`, `net_tx_errors`. Skip loopback (`lo`).

- [ ] **Step 1: fixture** `testdata/proc/net/dev` (procfs expects the 2 header lines + data):
```
Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000       10    0    0    0     0          0         0    1000       10    0    0    0     0       0          0
  eth0: 5000       50    2    0    0     0          0         0    6000       60    1    0    0     0       0          0
```

- [ ] **Step 2: failing test** `internal/collector/net_test.go`:
```go
package collector

import (
	"context"
	"testing"
)

func TestNetReadsFixtureSkipsLoopback(t *testing.T) {
	c := NewNet("n", "../../testdata/proc")
	got, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	byDev := map[string]map[string]float64{}
	for _, s := range got {
		if byDev[s.Device] == nil {
			byDev[s.Device] = map[string]float64{}
		}
		byDev[s.Device][s.Metric] = s.Value
	}
	if _, ok := byDev["lo"]; ok {
		t.Fatal("loopback must be skipped")
	}
	if byDev["eth0"]["net_rx_bytes"] != 5000 || byDev["eth0"]["net_tx_bytes"] != 6000 {
		t.Fatalf("eth0 bytes wrong: %+v", byDev["eth0"])
	}
	if byDev["eth0"]["net_rx_errors"] != 2 || byDev["eth0"]["net_tx_errors"] != 1 {
		t.Fatalf("eth0 errors wrong: %+v", byDev["eth0"])
	}
}
```

- [ ] **Step 3: run — FAIL.**

- [ ] **Step 4: implement** `internal/collector/net.go`:
```go
package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/procfs"
	"github.com/KeiaiLab/nodevitals/internal/model"
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
			return model.Sample{Node: c.node, Tier: "core", Device: iface, Metric: metric, Value: float64(v), Timestamp: now}
		}
		out = append(out,
			mk("net_rx_bytes", line.RxBytes), mk("net_tx_bytes", line.TxBytes),
			mk("net_rx_errors", line.RxErrors), mk("net_tx_errors", line.TxErrors))
	}
	return out, nil
}
```

- [ ] **Step 5: run — PASS.** (Note: map iteration → sample order varies; the test indexes by device, order-independent. Good.)
- [ ] **Step 6: commit** `git commit -am "feat(collector): per-interface counters from /proc/net/dev"`

---

### Task 5: Disk collector (blockdevice)

**Files:** Create `internal/collector/disk.go`, `disk_test.go`, `testdata/proc/diskstats`.

**Interfaces:** `collector.NewDisk(node, procRoot, sysRoot string) Collector` (diskstats needs both roots via `blockdevice.NewFS(procRoot, sysRoot)`). Per-disk `Device:"<name>"`, metrics `disk_read_bytes`, `disk_write_bytes` (sectors×512), `disk_read_ios`, `disk_write_ios`. Skip partitions/loop/ram if trivially detectable (keep simple: emit all whole devices reported).

- [ ] **Step 1: fixture** `testdata/proc/diskstats` (standard 20-field format; sda: readIOs=100 readSectors=2000 writeIOs=50 writeSectors=1000):
```
   8       0 sda 100 0 2000 10 50 0 1000 5 0 20 15 0 0 0 0 0 0
```

- [ ] **Step 2: failing test** `internal/collector/disk_test.go`:
```go
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
```
(Create an empty `testdata/sys/` dir so `blockdevice.NewFS` doesn't error on a missing sysRoot; if it requires `<sys>/block`, the implementer creates `testdata/sys/block/`.)

- [ ] **Step 3: run — FAIL.**

- [ ] **Step 4: implement** `internal/collector/disk.go`:
```go
package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/procfs/blockdevice"
	"github.com/KeiaiLab/nodevitals/internal/model"
)

type diskCollector struct {
	node     string
	procRoot string
	sysRoot  string
}

// NewDisk reports per-disk IO counters from /proc/diskstats.
func NewDisk(node, procRoot, sysRoot string) Collector {
	return &diskCollector{node: node, procRoot: procRoot, sysRoot: sysRoot}
}

func (c *diskCollector) Name() string { return "disk" }

func (c *diskCollector) Collect(ctx context.Context) ([]model.Sample, error) {
	fs, err := blockdevice.NewFS(c.procRoot, c.sysRoot)
	if err != nil {
		return nil, fmt.Errorf("open blockdevice fs: %w", err)
	}
	stats, err := fs.ProcDiskstats()
	if err != nil {
		return nil, fmt.Errorf("read diskstats: %w", err)
	}
	now := time.Now().UTC()
	var out []model.Sample
	for _, d := range stats {
		mk := func(metric string, v float64) model.Sample {
			return model.Sample{Node: c.node, Tier: "core", Device: d.DeviceName, Metric: metric, Value: v, Timestamp: now}
		}
		out = append(out,
			mk("disk_read_bytes", float64(d.ReadSectors)*512),
			mk("disk_write_bytes", float64(d.WriteSectors)*512),
			mk("disk_read_ios", float64(d.ReadIOs)),
			mk("disk_write_ios", float64(d.WriteIOs)))
	}
	return out, nil
}
```
(Implementer: confirm `Diskstats` field names — may be `d.ReadIOs`/`d.ReadSectors` directly or nested under `d.IOStats`. Adjust field access to the installed procfs version; run `go doc github.com/prometheus/procfs/blockdevice Diskstats`.)

- [ ] **Step 5: run — PASS.**
- [ ] **Step 6: commit** `git commit -am "feat(collector): per-disk IO from /proc/diskstats"`

---

### Task 6: hwmon collector (hand-rolled sysfs)

procfs has no hwmon support. Hand-parse `<sysRoot>/class/hwmon/hwmon*/` reading `name`, `temp*_input` (millidegrees → °C), `fan*_input` (rpm).

**Files:** Create `internal/collector/hwmon.go`, `hwmon_test.go`, `testdata/sys/class/hwmon/hwmon0/{name,temp1_input}`.

**Interfaces:** `collector.NewHwmon(node, sysRoot string) Collector`. `Device:"<chip>"` (from `name`), metrics `temp_celsius` (label `sensor` via... keep simple: Device=`<chip>/temp1`), `fan_rpm`.

- [ ] **Step 1: fixtures**
```
testdata/sys/class/hwmon/hwmon0/name        → "coretemp\n"
testdata/sys/class/hwmon/hwmon0/temp1_input → "45000\n"   (45.0 °C)
```

- [ ] **Step 2: failing test** `internal/collector/hwmon_test.go`:
```go
package collector

import (
	"context"
	"testing"
)

func TestHwmonReadsTempFixture(t *testing.T) {
	c := NewHwmon("n", "../../testdata/sys")
	got, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
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
```

- [ ] **Step 3: run — FAIL.**

- [ ] **Step 4: implement** `internal/collector/hwmon.go`:
```go
package collector

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/KeiaiLab/nodevitals/internal/model"
)

type hwmonCollector struct {
	node    string
	sysRoot string
}

// NewHwmon reports temperature/fan sensors from <sysRoot>/class/hwmon. A missing
// hwmon tree yields zero samples (not an error) — many nodes have no sensors.
func NewHwmon(node, sysRoot string) Collector { return &hwmonCollector{node: node, sysRoot: sysRoot} }

func (c *hwmonCollector) Name() string { return "hwmon" }

func readTrim(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (c *hwmonCollector) Collect(ctx context.Context) ([]model.Sample, error) {
	base := filepath.Join(c.sysRoot, "class", "hwmon")
	chips, err := os.ReadDir(base)
	if err != nil {
		return nil, nil // no hwmon tree → no samples, not an error
	}
	now := time.Now().UTC()
	var out []model.Sample
	for _, chip := range chips {
		dir := filepath.Join(base, chip.Name())
		name, err := readTrim(filepath.Join(dir, "name"))
		if err != nil {
			name = chip.Name()
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			fn := e.Name()
			switch {
			case strings.HasPrefix(fn, "temp") && strings.HasSuffix(fn, "_input"):
				if v, err := readTrim(filepath.Join(dir, fn)); err == nil {
					if milli, err := strconv.ParseFloat(v, 64); err == nil {
						out = append(out, model.Sample{
							Node: c.node, Tier: "core",
							Device: name + "/" + strings.TrimSuffix(fn, "_input"),
							Metric: "temp_celsius", Value: milli / 1000.0, Timestamp: now,
						})
					}
				}
			case strings.HasPrefix(fn, "fan") && strings.HasSuffix(fn, "_input"):
				if v, err := readTrim(filepath.Join(dir, fn)); err == nil {
					if rpm, err := strconv.ParseFloat(v, 64); err == nil {
						out = append(out, model.Sample{
							Node: c.node, Tier: "core",
							Device: name + "/" + strings.TrimSuffix(fn, "_input"),
							Metric: "fan_rpm", Value: rpm, Timestamp: now,
						})
					}
				}
			}
		}
	}
	return out, nil
}
```

- [ ] **Step 5: run — PASS** (2 tests). `go test ./internal/collector/ -run TestHwmon -v`
- [ ] **Step 6: full collector-package test + commit**
```bash
go test ./internal/collector/ -v   # all collectors green
git commit -am "feat(collector): hwmon temp/fan sensors from /sys/class/hwmon"
```

---

### Task 7: Config sysRoot + wire collectors into main.go

**Files:** Modify `internal/config/config.go` (add `SysRoot`), `cmd/nodevitals/main.go` (register the 5 new collectors on the core tier).

**Interfaces:**
- Consumes: all new `collector.New*`. Config gains `SysRoot string` (yaml `sysRoot`, default `/sys`).

- [ ] **Step 1: config SysRoot** — in `internal/config/config.go`, add to `Config`: `SysRoot string \`yaml:"sysRoot"\`` and in `Load` after the ProcRoot default: `if c.SysRoot == "" { c.SysRoot = "/sys" }`. Add a test in `config_test.go` asserting the `/sys` default (mirror `TestLoadDefaultsProcRootAndInterval`).

- [ ] **Step 2: run config test** `go test ./internal/config/ -v` → PASS.

- [ ] **Step 3: register collectors** — in `cmd/nodevitals/main.go`, where the loadavg collector is registered, replace with the full core set (guard by tier so gpu/smart images don't register core twice — but for M2a the binary is core-only, register all):
```go
	var reg collector.Registry
	reg.Add(collector.NewLoadAvg(cfg.Node, cfg.ProcRoot))
	reg.Add(collector.NewCPU(cfg.Node, cfg.ProcRoot))
	reg.Add(collector.NewMem(cfg.Node, cfg.ProcRoot))
	reg.Add(collector.NewNet(cfg.Node, cfg.ProcRoot))
	reg.Add(collector.NewDisk(cfg.Node, cfg.ProcRoot, cfg.SysRoot))
	reg.Add(collector.NewHwmon(cfg.Node, cfg.SysRoot))
```

- [ ] **Step 4: build + full regression**
```bash
go build ./... && go vet ./... && go test ./...
```
Expected: all green.

- [ ] **Step 5: commit** `git commit -am "feat(cmd,config): sysRoot + register full core collector set"`

---

### Task 8: Chart (/sys mount) + config defaults + smoke

**Files:** Modify `deploy/chart/templates/daemonset.yaml` (mount /sys read-only for hwmon), `deploy/chart/templates/configmap.yaml` (procRoot/sysRoot already? add sysRoot), `deploy/chart/values.yaml` (default rules for new metrics optional).

- [ ] **Step 1: mount /sys** — in `daemonset.yaml` core tier, add a read-only hostPath for /sys mounted at `/host/sys`, and set the ConfigMap's `sysRoot: /host/sys`. Add to volumes:
```yaml
        - name: sys
          hostPath:
            path: /sys
```
and volumeMounts:
```yaml
            - name: sys
              mountPath: /host/sys
              readOnly: true
```
and in `configmap.yaml` config.yaml add `sysRoot: /host/sys` next to `procRoot: /host/proc`.

- [ ] **Step 2: helm render + kubeconform**
```bash
helm template nv deploy/chart | grep -c "kind: DaemonSet"          # 1
helm template nv deploy/chart | grep -E "host/sys|host/proc"        # both present
helm template nv deploy/chart | kubeconform -strict -summary        # Valid
```

- [ ] **Step 3: real-binary smoke** (build binary, point at fixture proc+sys, verify new metrics appear):
```bash
SMOKE=$(mktemp -d)
cp -r testdata/proc "$SMOKE/proc"; cp -r testdata/sys "$SMOKE/sys"
cat > "$SMOKE/config.yaml" <<EOF
node: m2a-smoke
tier: core
intervalSeconds: 1
procRoot: $SMOKE/proc
sysRoot: $SMOKE/sys
sinks:
  metrics: { enabled: true, listenAddr: ":19848" }
rules: []
EOF
go build -o "$SMOKE/nodevitals" ./cmd/nodevitals
"$SMOKE/nodevitals" -config "$SMOKE/config.yaml" &
PID=$!
for i in $(seq 8); do curl -sf localhost:19848/healthz >/dev/null 2>&1 && break; sleep 0.5; done
echo "--- metrics ---"
curl -sf localhost:19848/metrics | grep -E "nodevitals_hw_(mem_total_bytes|temp_celsius|net_rx_bytes)" | head
kill $PID 2>/dev/null; rm -rf "$SMOKE"
```
Expected: `nodevitals_hw_mem_total_bytes`, `nodevitals_hw_temp_celsius`, `nodevitals_hw_net_rx_bytes` lines present (cpu_util needs 2 ticks so may appear on the 2nd scrape — acceptable).

- [ ] **Step 4: final regression + commit**
```bash
go build ./... && go vet ./... && go test ./... && helm template nv deploy/chart | kubeconform -strict -summary && echo "M2a GREEN"
git commit -am "feat(chart): mount /sys for hwmon + sysRoot config"
```

---

## Self-Review (작성자 체크 — 완료)

**1. Spec coverage (M2 design §2 core tier → tasks):** loadavg 이관(T1) / cpu-util(T2) / mem(T3) / net(T4) / disk(T5) / hwmon 손수(T6) / procfs 채택·root 주입(전 태스크) / 배포 배선(T7·T8). M2 design §2 표 전 항목 매핑. GPU/SMART = M2b/M2c(범위 밖).

**2. Placeholder scan:** 코드 완비. 단 procfs 필드명 3곳(`Stat.CPUTotal`/`Meminfo.*Bytes`/`Diskstats` 필드)은 "구현 시 `go doc` 확인" 명시 — 이는 라이브러리 버전 확인 지침(플레이스홀더 아님, M1 Context7 패턴 정합). Task 2 의 `statFile` seam 은 Step 4 note 로 "두 fixture 디렉터리" 방식으로 대체 지시(설계 명확).

**3. Type consistency:** 모든 콜렉터 `NewX(node, root...) Collector` + `Collect(ctx)([]model.Sample,error)` (M1 인터페이스 정합). Disk 만 `(node, procRoot, sysRoot)` — blockdevice 이중 루트라 정당. hwmon 은 `(node, sysRoot)`. config `SysRoot`(T7) → disk/hwmon 소비(T7 main). 전 Sample `Tier:"core"`.

**갭(의도적):** cpu-util 첫 tick baseline(이벤트 없음)은 정상. disk 파티션/loop 필터링은 최소(전 디바이스 방출) — 후속 개선. hwmon in_volts 생략(temp/fan 우선). 전부 M2a 범위 내 YAGNI.
