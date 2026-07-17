# nodevitals Foundation (Walking Skeleton) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 전체 파이프라인(수집 → 이벤트 판정 → webhook/REST/metrics 전달 → Helm 배포)을 단일 core 콜렉터(loadavg)로 수직 관통하는, 배포·테스트 가능한 최소 에이전트를 만든다.

**Architecture:** Go 단일 정적 바이너리. `collector`(하드웨어 read) → `event.Engine`(순수 함수 상태전이 판정) → `sink`(webhook/metrics) 파이프라인을 `agent`가 조립. 모든 하드웨어 접근은 경로 주입(fixture-root)으로 추상화해 하드웨어 0대에서 결정론 테스트. Helm 차트는 core tier DaemonSet 하나를 렌더.

**Tech Stack:** Go 1.26 · `github.com/prometheus/client_golang`(/metrics) · `gopkg.in/yaml.v3`(config) · 표준 `net/http`(webhook·REST) · CloudEvents 1.0 엔벨로프 자작 + Standard Webhooks HMAC-SHA256 · Helm 3.

## Global Constraints

- **Go module path**: `github.com/nodevitals/nodevitals` (GitHub org `nodevitals` 가용 실측 완료). 모든 import 이 경로 기준.
- **Go 버전**: 1.26+ (로컬 검증 = go1.26.5). `go.mod` 의 `go` 지시어 = `1.26`.
- **라이선스**: Apache-2.0 (모든 소스 파일 헤더 불요 — LICENSE 파일로 충분, node_exporter 관례).
- **이름**: 프로젝트·바이너리·모듈 전부 `nodevitals`. 메트릭 네임스페이스 접두 = `nodevitals_`.
- **플랫폼**: `linux` 런타임 (에이전트는 리눅스 노드 전용). 개발·테스트는 macOS/linux 무관하게 통과해야 함 → **`/proc`·`/sys` 직접 접근 금지, 반드시 경로 주입**. (macOS 에 `/proc` 없음 — fixture 필수 이유)
- **아키텍처**: amd64 + arm64 (ADR-0001 OSS 예외). Go 크로스컴파일로 처리.
- **CI**: GitHub Actions 금지 (거버넌스 §2.3) → 로컬 게이트 = Makefile + (후속) lefthook. GitHub Actions 도입은 별도 ADR 필요 (본 계획 밖).
- **테스트**: 하드웨어·네트워크 실접근 0. `sleep` 동기화 금지 (주입 clock 또는 채널 동기). 표준 `testing` + `httptest`.
- **커밋**: Conventional Commits, 각 Task 끝에 커밋. 원격 push 없음 (repo 원격 미설정 — 로컬 커밋만).

---

## File Structure (M1 종료 시점)

```
nodevitals/                      (repo root — Task 1 에서 voltra→nodevitals 재구성)
  go.mod  go.sum
  LICENSE                        Apache-2.0
  README.md                      최소 소개
  Makefile                       test/lint/build/docker 로컬 게이트
  Dockerfile                     Go 멀티스테이지 → distroless/static
  .dockerignore  .gitignore
  cmd/nodevitals/main.go         entrypoint
  internal/
    config/config.go  config_test.go
    model/model.go  model_test.go
    collector/collector.go  collector_test.go  loadavg.go  loadavg_test.go
    event/event.go  event_test.go
    sink/sink.go  webhook.go  webhook_test.go  metrics.go  metrics_test.go
    queue/queue.go  queue_test.go
    agent/agent.go  agent_test.go
    httpapi/server.go  server_test.go
  deploy/chart/
    Chart.yaml  values.yaml
    templates/_helpers.tpl  daemonset.yaml  configmap.yaml
  docs/kb/adr/0001-arm64-oss-exception.md
  testdata/proc/loadavg          fixture
```

**의존 순서** (Task 는 이 순서로 — 뒤 Task 가 앞 Task 산출물 소비):
model → config → collector(+loadavg) → event → sink(webhook) → sink(metrics) → queue → httpapi → agent → chart → packaging.

---

### Task 1: 리포 재구성 (Python 제거 + Go 초기화 + 라이선스 + ADR)

Python 스캐폴드를 지우고 Go 모듈로 재출발한다. 커밋 0 상태라 히스토리 손실 없음.

**Files:**
- Delete: `main.py`, `pyproject.toml`, `uv.lock`, `.python-version`, `.venv/`
- Replace: `Dockerfile`, `.dockerignore`, `.gitignore`
- Create: `go.mod`, `LICENSE`, `README.md`, `docs/kb/adr/0001-arm64-oss-exception.md`

- [ ] **Step 1: Python 스캐폴드 제거**

```bash
cd /Users/phil/oss/voltra
rm -rf main.py pyproject.toml uv.lock .python-version .venv
```

- [ ] **Step 2: Go 모듈 초기화**

```bash
go mod init github.com/nodevitals/nodevitals
```

기대: `go.mod` 생성, 내용에 `module github.com/nodevitals/nodevitals` + `go 1.26`.

- [ ] **Step 3: `.gitignore` 교체 (Go)**

Create `.gitignore`:
```
# Binaries
/nodevitals
/dist/
*.exe
# Test
*.out
coverage.*
# Go
/vendor/
# Editors
.DS_Store
```

- [ ] **Step 4: LICENSE (Apache-2.0)**

Create `LICENSE` — 표준 Apache License 2.0 전문. 공식 텍스트를 그대로 넣는다:
```bash
curl -fsSL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE
```
기대: `LICENSE` 첫 줄에 "Apache License", "Version 2.0" 포함. (네트워크 불가 시 https://www.apache.org/licenses/LICENSE-2.0.txt 전문 수기 복사)

- [ ] **Step 5: ADR-0001 (arm64 OSS 예외)**

Create `docs/kb/adr/0001-arm64-oss-exception.md`:
```markdown
# ADR-0001: arm64 멀티아키텍처 — OSS 예외

- 상태: Accepted
- 날짜: 2026-07-17

## 맥락
거버넌스 §2.3 은 컨테이너 이미지를 `linux/amd64` 단일 아키텍처로 강제하고 멀티아키텍처를 금지한다. 이는 내부 클러스터 자산 전제의 규칙이다.

## 결정
nodevitals 는 공개 OSS 노드 에이전트다. 비교군(node-exporter/dcgm-exporter/telegraf/netdata/grafana-agent) 전부 arm64 를 배포하며, dcgm-exporter 는 GH200/Grace-Hopper/Jetson 때문에 arm64 를 낸다. 따라서 nodevitals 는 **amd64 + arm64** 이미지를 배포한다. §2.8(OSS = GitHub canonical)에 근거한 예외.

## 결과
- Go 크로스컴파일(`GOOS=linux GOARCH={amd64,arm64}`)로 단일 소스에서 양 아키 바이너리 생성.
- 멀티아치 이미지 매니페스트 발행은 릴리스 단계 작업(후속 마일스톤).
- 내부 클러스터 배포 대상이 아니므로 §2.3 의 원래 우려(내부 빌드 SPOF)와 무관.
```

- [ ] **Step 6: README (최소)**

Create `README.md`:
```markdown
# nodevitals

Unified hardware telemetry agent for Kubernetes nodes — collects deep hardware
state (CPU, memory, GPU, disk/SMART, sensors), evaluates state-transition events,
and delivers via webhook push, REST snapshot, and Prometheus `/metrics`. One agent
and one Helm chart replace the node-exporter + dcgm-exporter + smartctl-exporter wiring.

Status: early development (v0.1 walking skeleton).

License: Apache-2.0
```

- [ ] **Step 7: Dockerfile (Go 멀티스테이지 → distroless/static)**

Replace `Dockerfile`:
```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.26-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/nodevitals ./cmd/nodevitals

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/nodevitals /nodevitals
USER nonroot
ENTRYPOINT ["/nodevitals"]
```

Replace `.dockerignore`:
```
.git
.venv
dist/
docs/
*.md
testdata/
```

- [ ] **Step 8: 빌드 스모크 (아직 main 없음 → 다음 Task 후 통과. 지금은 go.mod 검증만)**

Run: `go mod verify`
Expected: `all modules verified`

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "chore: reset scaffold to Go module + Apache-2.0 + ADR-0001 arm64"
```

---

### Task 2: 도메인 타입 (Sample, Event)

**Files:**
- Create: `internal/model/model.go`, `internal/model/model_test.go`

**Interfaces:**
- Produces: `model.Sample{Node,Tier,Device,Metric string; Value float64; Labels map[string]string; Timestamp time.Time}`, `model.Event{ID,Node,Tier,Device,Condition,Phase,Severity string; Seq uint64; StartedAt,EndedAt time.Time; Detail map[string]any}`, 상수 `model.PhaseEnter="ENTER"`, `model.PhaseExit="EXIT"`, `model.SevInfo/SevWarning/SevCritical`.

- [ ] **Step 1: 실패 테스트 작성**

Create `internal/model/model_test.go`:
```go
package model

import "testing"

func TestFingerprintStableForSameKey(t *testing.T) {
	a := Event{Node: "n1", Tier: "core", Device: "cpu", Condition: "load_high"}
	b := Event{Node: "n1", Tier: "core", Device: "cpu", Condition: "load_high", Seq: 99}
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatalf("fingerprint must ignore volatile fields: %s != %s", a.Fingerprint(), b.Fingerprint())
	}
}

func TestFingerprintDiffersByCondition(t *testing.T) {
	a := Event{Node: "n1", Tier: "core", Device: "cpu", Condition: "load_high"}
	b := Event{Node: "n1", Tier: "core", Device: "cpu", Condition: "temp_high"}
	if a.Fingerprint() == b.Fingerprint() {
		t.Fatal("different condition must yield different fingerprint")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/model/ -run TestFingerprint -v`
Expected: FAIL — `Event` / `Fingerprint` undefined (컴파일 에러).

- [ ] **Step 3: 최소 구현**

Create `internal/model/model.go`:
```go
// Package model defines the core data types shared across nodevitals.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const (
	PhaseEnter = "ENTER"
	PhaseExit  = "EXIT"

	SevInfo     = "info"
	SevWarning  = "warning"
	SevCritical = "critical"
)

// Sample is one hardware measurement.
type Sample struct {
	Node      string            `json:"node"`
	Tier      string            `json:"tier"`
	Device    string            `json:"device"`
	Metric    string            `json:"metric"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// Event is a hardware state transition.
type Event struct {
	ID        string         `json:"id"`
	Node      string         `json:"node"`
	Tier      string         `json:"tier"`
	Device    string         `json:"device"`
	Condition string         `json:"condition"`
	Phase     string         `json:"phase"`
	Severity  string         `json:"severity"`
	Seq       uint64         `json:"seq"`
	StartedAt time.Time      `json:"started_at"`
	EndedAt   time.Time      `json:"ended_at,omitempty"`
	Detail    map[string]any `json:"detail,omitempty"`
}

// Fingerprint is a stable identity for an event's (node,tier,device,condition),
// ignoring volatile fields (seq, timestamps). Used for dedup and idempotency.
func (e Event) Fingerprint() string {
	h := sha256.Sum256([]byte(e.Node + "\x00" + e.Tier + "\x00" + e.Device + "\x00" + e.Condition))
	return hex.EncodeToString(h[:8])
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/model/ -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/model/
git commit -m "feat(model): Sample and Event types with stable fingerprint"
```

---

### Task 3: Config 스키마 + 로더

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`

**Interfaces:**
- Consumes: (없음)
- Produces: `config.Config{Node string; Tier string; IntervalSeconds int; ProcRoot string; Rules []Rule; Sinks SinksConfig}`, `config.Rule{Metric,Device,Condition,Severity string; Threshold float64; EnterFor,ExitFor int}`, `config.SinksConfig{Webhook []WebhookConfig; Metrics MetricsConfig}`, `config.WebhookConfig{URL,Secret string}`, `config.MetricsConfig{Enabled bool; ListenAddr string}`, `func Load(path string) (Config, error)`, `func (c Config) Interval() time.Duration`.

- [ ] **Step 1: 실패 테스트 작성**

Create `internal/config/config_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadParsesYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	os.WriteFile(p, []byte(`
node: test-node
tier: core
intervalSeconds: 5
procRoot: /custom/proc
rules:
  - metric: load1
    device: cpu
    condition: load_high
    severity: warning
    threshold: 4.0
    enterFor: 2
    exitFor: 2
sinks:
  webhook:
    - url: https://backend.example/hook
      secret: shh
  metrics:
    enabled: true
    listenAddr: ":9847"
`), 0o644)

	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Node != "test-node" || c.Tier != "core" {
		t.Fatalf("bad node/tier: %+v", c)
	}
	if c.Interval() != 5*time.Second {
		t.Fatalf("interval = %v, want 5s", c.Interval())
	}
	if len(c.Rules) != 1 || c.Rules[0].Threshold != 4.0 {
		t.Fatalf("bad rules: %+v", c.Rules)
	}
	if len(c.Sinks.Webhook) != 1 || c.Sinks.Webhook[0].URL != "https://backend.example/hook" {
		t.Fatalf("bad webhook: %+v", c.Sinks.Webhook)
	}
	if !c.Sinks.Metrics.Enabled {
		t.Fatal("metrics should be enabled")
	}
}

func TestLoadDefaultsProcRootAndInterval(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	os.WriteFile(p, []byte("node: n\ntier: core\n"), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ProcRoot != "/proc" {
		t.Fatalf("procRoot default = %q, want /proc", c.ProcRoot)
	}
	if c.Interval() != 15*time.Second {
		t.Fatalf("interval default = %v, want 15s", c.Interval())
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `Load` / `Config` undefined.

- [ ] **Step 3: yaml 의존 추가**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 4: 최소 구현**

Create `internal/config/config.go`:
```go
// Package config loads the nodevitals agent configuration.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Rule struct {
	Metric    string  `yaml:"metric"`
	Device    string  `yaml:"device"`
	Condition string  `yaml:"condition"`
	Severity  string  `yaml:"severity"`
	Threshold float64 `yaml:"threshold"`
	EnterFor  int     `yaml:"enterFor"`
	ExitFor   int     `yaml:"exitFor"`
}

type WebhookConfig struct {
	URL    string `yaml:"url"`
	Secret string `yaml:"secret"`
}

type MetricsConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ListenAddr string `yaml:"listenAddr"`
}

type SinksConfig struct {
	Webhook []WebhookConfig `yaml:"webhook"`
	Metrics MetricsConfig   `yaml:"metrics"`
}

type Config struct {
	Node            string      `yaml:"node"`
	Tier            string      `yaml:"tier"`
	IntervalSeconds int         `yaml:"intervalSeconds"`
	ProcRoot        string      `yaml:"procRoot"`
	Rules           []Rule      `yaml:"rules"`
	Sinks           SinksConfig `yaml:"sinks"`
}

// Interval returns the collection interval, defaulting to 15s.
func (c Config) Interval() time.Duration {
	if c.IntervalSeconds <= 0 {
		return 15 * time.Second
	}
	return time.Duration(c.IntervalSeconds) * time.Second
}

// Load reads and parses a YAML config file, applying defaults.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if c.ProcRoot == "" {
		c.ProcRoot = "/proc"
	}
	return c, nil
}
```

- [ ] **Step 5: 통과 확인**

Run: `go test ./internal/config/ -v`
Expected: PASS (2 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat(config): YAML config schema and loader with defaults"
```

---

### Task 4: Collector 인터페이스 + loadavg 콜렉터 (fixture-root)

**Files:**
- Create: `internal/collector/collector.go`, `internal/collector/collector_test.go`, `internal/collector/loadavg.go`, `internal/collector/loadavg_test.go`, `testdata/proc/loadavg`

**Interfaces:**
- Consumes: `model.Sample`
- Produces: `collector.Collector` interface `{ Name() string; Collect(ctx context.Context) ([]model.Sample, error) }`, `collector.Registry{}` with `Add(Collector)` and `CollectAll(ctx) []model.Sample`, `collector.NewLoadAvg(node, procRoot string) Collector`.

- [ ] **Step 1: fixture 작성**

Create `testdata/proc/loadavg`:
```
0.52 0.58 0.59 2/1234 56789
```

- [ ] **Step 2: 실패 테스트 작성 (loadavg)**

Create `internal/collector/loadavg_test.go`:
```go
package collector

import (
	"context"
	"testing"
)

func TestLoadAvgReadsFixture(t *testing.T) {
	c := NewLoadAvg("test-node", "../../testdata/proc")
	samples, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("want 1 sample, got %d", len(samples))
	}
	s := samples[0]
	if s.Metric != "load1" || s.Value != 0.52 {
		t.Fatalf("bad sample: %+v", s)
	}
	if s.Node != "test-node" || s.Tier != "core" || s.Device != "cpu" {
		t.Fatalf("bad identity: %+v", s)
	}
}

func TestLoadAvgMissingFileErrors(t *testing.T) {
	c := NewLoadAvg("n", "/nonexistent")
	if _, err := c.Collect(context.Background()); err == nil {
		t.Fatal("expected error for missing loadavg")
	}
}
```

- [ ] **Step 3: 실패 확인**

Run: `go test ./internal/collector/ -run TestLoadAvg -v`
Expected: FAIL — `NewLoadAvg` undefined.

- [ ] **Step 4: 인터페이스 + loadavg 구현**

Create `internal/collector/collector.go`:
```go
// Package collector reads hardware state. Each collector covers one domain and
// performs read-only access; all filesystem roots are injected for testability.
package collector

import (
	"context"

	"github.com/nodevitals/nodevitals/internal/model"
)

// Collector samples one hardware domain.
type Collector interface {
	Name() string
	Collect(ctx context.Context) ([]model.Sample, error)
}

// Registry holds the active collectors for a tier.
type Registry struct {
	collectors []Collector
}

func (r *Registry) Add(c Collector) { r.collectors = append(r.collectors, c) }

// CollectAll runs every collector; a failing collector is skipped (its error is
// dropped here — callers relying on liveness use agent-level self-metrics).
func (r *Registry) CollectAll(ctx context.Context) []model.Sample {
	var out []model.Sample
	for _, c := range r.collectors {
		s, err := c.Collect(ctx)
		if err != nil {
			continue
		}
		out = append(out, s...)
	}
	return out
}
```

Create `internal/collector/loadavg.go`:
```go
package collector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nodevitals/nodevitals/internal/model"
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
	b, err := os.ReadFile(filepath.Join(l.procRoot, "loadavg"))
	if err != nil {
		return nil, fmt.Errorf("read loadavg: %w", err)
	}
	fields := strings.Fields(string(b))
	if len(fields) < 1 {
		return nil, fmt.Errorf("malformed loadavg: %q", string(b))
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil, fmt.Errorf("parse load1: %w", err)
	}
	return []model.Sample{{
		Node: l.node, Tier: "core", Device: "cpu", Metric: "load1",
		Value: v, Timestamp: time.Now().UTC(),
	}}, nil
}
```

- [ ] **Step 5: 통과 확인 (loadavg)**

Run: `go test ./internal/collector/ -run TestLoadAvg -v`
Expected: PASS (2 tests).

- [ ] **Step 6: Registry 테스트 작성**

Create `internal/collector/collector_test.go`:
```go
package collector

import (
	"context"
	"errors"
	"testing"

	"github.com/nodevitals/nodevitals/internal/model"
)

type stubCollector struct {
	name    string
	samples []model.Sample
	err     error
}

func (s stubCollector) Name() string { return s.name }
func (s stubCollector) Collect(context.Context) ([]model.Sample, error) {
	return s.samples, s.err
}

func TestRegistrySkipsFailingCollector(t *testing.T) {
	var r Registry
	r.Add(stubCollector{name: "ok", samples: []model.Sample{{Metric: "a"}}})
	r.Add(stubCollector{name: "bad", err: errors.New("boom")})
	r.Add(stubCollector{name: "ok2", samples: []model.Sample{{Metric: "b"}}})

	got := r.CollectAll(context.Background())
	if len(got) != 2 {
		t.Fatalf("want 2 samples (failing skipped), got %d", len(got))
	}
}
```

- [ ] **Step 7: 통과 확인 (전체 패키지)**

Run: `go test ./internal/collector/ -v`
Expected: PASS (3 tests).

- [ ] **Step 8: Commit**

```bash
git add internal/collector/ testdata/
git commit -m "feat(collector): Collector interface, Registry, loadavg with fixture-root"
```

---

### Task 5: Event Engine (순수 함수 상태전이 + 히스테리시스)

가장 중요한 컴포넌트. 하드웨어 없이 완전 테스트됨. Sample 스트림을 상태전이 Event 로 변환.

**Files:**
- Create: `internal/event/event.go`, `internal/event/event_test.go`

**Interfaces:**
- Consumes: `model.Sample`, `model.Event`, `config.Rule`
- Produces: `event.NewEngine(node string, rules []config.Rule) *Engine`, `func (e *Engine) Evaluate(samples []model.Sample) []model.Event`. Engine 은 룰별 내부 상태(연속 초과/미달 카운트, 활성 여부, seq) 보유. ENTER 는 `EnterFor` 연속 초과 시, EXIT 는 `ExitFor` 연속 미달 시 발화.

- [ ] **Step 1: 실패 테스트 작성**

Create `internal/event/event_test.go`:
```go
package event

import (
	"testing"
	"time"

	"github.com/nodevitals/nodevitals/internal/config"
	"github.com/nodevitals/nodevitals/internal/model"
)

func rule() config.Rule {
	return config.Rule{
		Metric: "load1", Device: "cpu", Condition: "load_high",
		Severity: "warning", Threshold: 4.0, EnterFor: 2, ExitFor: 2,
	}
}

func sample(v float64) []model.Sample {
	return []model.Sample{{Node: "n", Tier: "core", Device: "cpu", Metric: "load1", Value: v, Timestamp: time.Now()}}
}

func TestEnterFiresAfterEnterForConsecutiveBreaches(t *testing.T) {
	e := NewEngine("n", []config.Rule{rule()})
	if ev := e.Evaluate(sample(5)); len(ev) != 0 {
		t.Fatalf("breach 1/2 must not fire, got %d", len(ev))
	}
	ev := e.Evaluate(sample(5))
	if len(ev) != 1 || ev[0].Phase != model.PhaseEnter {
		t.Fatalf("breach 2/2 must ENTER, got %+v", ev)
	}
	if ev[0].Condition != "load_high" || ev[0].Severity != "warning" || ev[0].Seq != 1 {
		t.Fatalf("bad enter event: %+v", ev[0])
	}
}

func TestNoDuplicateEnterWhileActive(t *testing.T) {
	e := NewEngine("n", []config.Rule{rule()})
	e.Evaluate(sample(5))
	e.Evaluate(sample(5)) // ENTER
	if ev := e.Evaluate(sample(6)); len(ev) != 0 {
		t.Fatalf("must not re-ENTER while active, got %+v", ev)
	}
}

func TestExitFiresAfterExitForConsecutiveClears(t *testing.T) {
	e := NewEngine("n", []config.Rule{rule()})
	e.Evaluate(sample(5))
	e.Evaluate(sample(5)) // ENTER (seq 1)
	if ev := e.Evaluate(sample(1)); len(ev) != 0 {
		t.Fatalf("clear 1/2 must not EXIT, got %+v", ev)
	}
	ev := e.Evaluate(sample(1))
	if len(ev) != 1 || ev[0].Phase != model.PhaseExit {
		t.Fatalf("clear 2/2 must EXIT, got %+v", ev)
	}
	if ev[0].Seq != 2 || ev[0].EndedAt.IsZero() {
		t.Fatalf("exit must carry seq=2 and EndedAt: %+v", ev[0])
	}
}

func TestHysteresisResetsClearCountOnRebreapch(t *testing.T) {
	e := NewEngine("n", []config.Rule{rule()})
	e.Evaluate(sample(5))
	e.Evaluate(sample(5)) // ENTER
	e.Evaluate(sample(1)) // clear 1/2
	e.Evaluate(sample(9)) // breach again → clear count resets
	if ev := e.Evaluate(sample(1)); len(ev) != 0 {
		t.Fatalf("clear count must have reset; single clear must not EXIT, got %+v", ev)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/event/ -v`
Expected: FAIL — `NewEngine` undefined.

- [ ] **Step 3: 최소 구현**

Create `internal/event/event.go`:
```go
// Package event turns a stream of samples into hardware state-transition events.
// It is deterministic and hardware-free: give it samples, get events.
package event

import (
	"time"

	"github.com/nodevitals/nodevitals/internal/config"
	"github.com/nodevitals/nodevitals/internal/model"
)

type ruleState struct {
	rule       config.Rule
	active     bool
	breachRun  int
	clearRun   int
	seq        uint64
	startedAt  time.Time
}

// Engine evaluates rules against samples, holding per-rule hysteresis state.
type Engine struct {
	node  string
	rules map[string]*ruleState // key: condition
}

func NewEngine(node string, rules []config.Rule) *Engine {
	m := make(map[string]*ruleState, len(rules))
	for _, r := range rules {
		m[r.Condition] = &ruleState{rule: r}
	}
	return &Engine{node: node, rules: m}
}

// Evaluate returns any state-transition events triggered by these samples.
func (e *Engine) Evaluate(samples []model.Sample) []model.Event {
	var out []model.Event
	now := time.Now().UTC()
	for _, s := range samples {
		for _, st := range e.rules {
			if st.rule.Metric != s.Metric || st.rule.Device != s.Device {
				continue
			}
			breached := s.Value > st.rule.Threshold
			if breached {
				st.breachRun++
				st.clearRun = 0
			} else {
				st.clearRun++
				st.breachRun = 0
			}

			enterFor := max1(st.rule.EnterFor)
			exitFor := max1(st.rule.ExitFor)

			if !st.active && st.breachRun >= enterFor {
				st.active = true
				st.seq++
				st.startedAt = now
				out = append(out, model.Event{
					Node: e.node, Tier: s.Tier, Device: s.Device,
					Condition: st.rule.Condition, Phase: model.PhaseEnter,
					Severity: st.rule.Severity, Seq: st.seq, StartedAt: now,
					Detail: map[string]any{"value": s.Value, "threshold": st.rule.Threshold},
				})
			} else if st.active && st.clearRun >= exitFor {
				st.active = false
				st.seq++
				out = append(out, model.Event{
					Node: e.node, Tier: s.Tier, Device: s.Device,
					Condition: st.rule.Condition, Phase: model.PhaseExit,
					Severity: st.rule.Severity, Seq: st.seq,
					StartedAt: st.startedAt, EndedAt: now,
					Detail: map[string]any{"value": s.Value, "threshold": st.rule.Threshold},
				})
			}
		}
	}
	for i := range out {
		out[i].ID = out[i].Fingerprint()
	}
	return out
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/event/ -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/event/
git commit -m "feat(event): pure-function state-transition engine with hysteresis"
```

---

### Task 6: Sink 인터페이스 + Webhook (CloudEvents + HMAC)

**Files:**
- Create: `internal/sink/sink.go`, `internal/sink/webhook.go`, `internal/sink/webhook_test.go`

**Interfaces:**
- Consumes: `model.Event`, `config.WebhookConfig`
- Produces: `sink.Sink` interface `{ Name() string; EmitEvents(ctx, []model.Event) error }`, `sink.CloudEvent` 엔벨로프 struct + `sink.WrapEvent(model.Event) CloudEvent`, `sink.NewWebhook(cfg config.WebhookConfig, client *http.Client) *Webhook`, `sink.Sign(secret string, body []byte) string` (HMAC-SHA256 → `sha256=<hex>`).

- [ ] **Step 1: 실패 테스트 작성**

Create `internal/sink/webhook_test.go`:
```go
package sink

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nodevitals/nodevitals/internal/config"
	"github.com/nodevitals/nodevitals/internal/model"
)

func TestWebhookPostsSignedCloudEvent(t *testing.T) {
	var gotBody []byte
	var gotSig, gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get("Webhook-Signature")
		gotType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhook(config.WebhookConfig{URL: srv.URL, Secret: "shh"}, srv.Client())
	ev := model.Event{Node: "n", Tier: "core", Device: "cpu", Condition: "load_high", Phase: model.PhaseEnter}
	ev.ID = ev.Fingerprint()

	if err := wh.EmitEvents(context.Background(), []model.Event{ev}); err != nil {
		t.Fatalf("EmitEvents: %v", err)
	}

	if gotType != "application/cloudevents+json" {
		t.Fatalf("content-type = %q", gotType)
	}
	if gotSig != Sign("shh", gotBody) {
		t.Fatalf("signature mismatch: %q", gotSig)
	}
	var ce CloudEvent
	if err := json.Unmarshal(gotBody, &ce); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if ce.SpecVersion != "1.0" || ce.Type != "com.nodevitals.hw.event.v1" {
		t.Fatalf("bad envelope: %+v", ce)
	}
}

func TestWebhookNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	wh := NewWebhook(config.WebhookConfig{URL: srv.URL, Secret: "s"}, srv.Client())
	err := wh.EmitEvents(context.Background(), []model.Event{{Condition: "x"}})
	if err == nil {
		t.Fatal("expected error on 500")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/sink/ -v`
Expected: FAIL — `NewWebhook` / `CloudEvent` / `Sign` undefined.

- [ ] **Step 3: sink 인터페이스 + webhook 구현**

Create `internal/sink/sink.go`:
```go
// Package sink delivers events and samples to destinations.
package sink

import (
	"context"

	"github.com/nodevitals/nodevitals/internal/model"
)

// Sink delivers events to one destination.
type Sink interface {
	Name() string
	EmitEvents(ctx context.Context, events []model.Event) error
}
```

Create `internal/sink/webhook.go`:
```go
package sink

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nodevitals/nodevitals/internal/config"
	"github.com/nodevitals/nodevitals/internal/model"
)

// CloudEvent is a CloudEvents 1.0 structured-mode envelope.
type CloudEvent struct {
	SpecVersion     string      `json:"specversion"`
	Type            string      `json:"type"`
	Source          string      `json:"source"`
	ID              string      `json:"id"`
	Time            time.Time   `json:"time"`
	DataContentType string      `json:"datacontenttype"`
	Data            model.Event `json:"data"`
}

// WrapEvent builds a CloudEvents envelope around a hardware event.
func WrapEvent(ev model.Event) CloudEvent {
	return CloudEvent{
		SpecVersion:     "1.0",
		Type:            "com.nodevitals.hw.event.v1",
		Source:          "nodevitals/" + ev.Node,
		ID:              ev.ID,
		Time:            time.Now().UTC(),
		DataContentType: "application/json",
		Data:            ev,
	}
}

// Sign returns the Standard Webhooks HMAC-SHA256 signature of body.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Webhook posts CloudEvents to a customer backend endpoint.
type Webhook struct {
	cfg    config.WebhookConfig
	client *http.Client
}

func NewWebhook(cfg config.WebhookConfig, client *http.Client) *Webhook {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Webhook{cfg: cfg, client: client}
}

func (w *Webhook) Name() string { return "webhook:" + w.cfg.URL }

func (w *Webhook) EmitEvents(ctx context.Context, events []model.Event) error {
	for _, ev := range events {
		body, err := json.Marshal(WrapEvent(ev))
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.cfg.URL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/cloudevents+json")
		req.Header.Set("Webhook-Id", ev.ID)
		req.Header.Set("Webhook-Signature", Sign(w.cfg.Secret, body))
		resp, err := w.client.Do(req)
		if err != nil {
			return fmt.Errorf("post webhook: %w", err)
		}
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("webhook %s returned %d", w.cfg.URL, resp.StatusCode)
		}
	}
	return nil
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/sink/ -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/sink/sink.go internal/sink/webhook.go internal/sink/webhook_test.go
git commit -m "feat(sink): webhook sink with CloudEvents envelope and HMAC signing"
```

---

### Task 7: Metrics Sink (Prometheus /metrics)

최신 Sample 을 Prometheus gauge 로 노출하는 custom Collector.

**Files:**
- Create: `internal/sink/metrics.go`, `internal/sink/metrics_test.go`

**Interfaces:**
- Consumes: `model.Sample`, prometheus custom Collector 패턴 (Context7 확인: `Describe(ch)` + `Collect(ch)` + `prometheus.MustNewConstMetric`)
- Produces: `sink.NewMetrics() *Metrics`, `func (m *Metrics) Update(samples []model.Sample)`, `func (m *Metrics) Describe(chan<- *prometheus.Desc)`, `func (m *Metrics) Collect(chan<- prometheus.Metric)` (prometheus.Collector 구현), `func (m *Metrics) Handler() http.Handler`.

- [ ] **Step 1: prometheus 의존 추가**

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

- [ ] **Step 2: 실패 테스트 작성**

Create `internal/sink/metrics_test.go`:
```go
package sink

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nodevitals/nodevitals/internal/model"
)

func TestMetricsExposesLatestSample(t *testing.T) {
	m := NewMetrics()
	m.Update([]model.Sample{
		{Node: "n1", Tier: "core", Device: "cpu", Metric: "load1", Value: 1.5},
	})

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	if !strings.Contains(body, `nodevitals_hw_load1`) {
		t.Fatalf("missing metric name in output:\n%s", body)
	}
	if !strings.Contains(body, `device="cpu"`) || !strings.Contains(body, `1.5`) {
		t.Fatalf("missing labels/value:\n%s", body)
	}
}

func TestMetricsUpdateReplacesSnapshot(t *testing.T) {
	m := NewMetrics()
	m.Update([]model.Sample{{Node: "n", Tier: "core", Device: "cpu", Metric: "load1", Value: 1}})
	m.Update([]model.Sample{{Node: "n", Tier: "core", Device: "cpu", Metric: "load1", Value: 9}})

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, _ := http.Get(srv.URL)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), "9") {
		t.Fatalf("expected updated value 9:\n%s", string(raw))
	}
}
```

- [ ] **Step 3: 실패 확인**

Run: `go test ./internal/sink/ -run TestMetrics -v`
Expected: FAIL — `NewMetrics` undefined.

- [ ] **Step 4: 구현**

Create `internal/sink/metrics.go`:
```go
package sink

import (
	"net/http"
	"sync"

	"github.com/nodevitals/nodevitals/internal/model"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics exposes the latest sample snapshot as Prometheus gauges. It implements
// prometheus.Collector, emitting const metrics on scrape from the held snapshot.
type Metrics struct {
	mu       sync.RWMutex
	snapshot []model.Sample
	reg      *prometheus.Registry
}

func NewMetrics() *Metrics {
	m := &Metrics{reg: prometheus.NewRegistry()}
	m.reg.MustRegister(m)
	return m
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
		desc := prometheus.NewDesc(
			"nodevitals_hw_"+s.Metric,
			"nodevitals hardware metric "+s.Metric,
			[]string{"node", "tier", "device"}, nil,
		)
		ch <- prometheus.MustNewConstMetric(
			desc, prometheus.GaugeValue, s.Value, s.Node, s.Tier, s.Device,
		)
	}
}

// Handler returns the /metrics HTTP handler.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{Registry: m.reg})
}
```

- [ ] **Step 5: 통과 확인**

Run: `go test ./internal/sink/ -v`
Expected: PASS (webhook 2 + metrics 2 = 4 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/sink/metrics.go internal/sink/metrics_test.go go.mod go.sum
git commit -m "feat(sink): Prometheus metrics sink exposing latest sample snapshot"
```

---

### Task 8: Delivery Queue (유계 + Full Jitter 백오프)

sink 앞단에서 재시도·백오프를 처리. 주입 clock 으로 결정론 테스트.

**Files:**
- Create: `internal/queue/queue.go`, `internal/queue/queue_test.go`

**Interfaces:**
- Consumes: `model.Event`, `sink.Sink`
- Produces: `queue.Backoff{Base, Max time.Duration}` with `func (b Backoff) For(attempt int, rnd float64) time.Duration` (Full Jitter: `random(0, min(Max, Base*2^attempt))`), `queue.DeliverWithRetry(ctx, s sink.Sink, events []model.Event, maxAttempts int, b Backoff, sleep func(time.Duration), rnd func() float64) error`.

- [ ] **Step 1: 실패 테스트 작성**

Create `internal/queue/queue_test.go`:
```go
package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nodevitals/nodevitals/internal/model"
)

func TestBackoffFullJitterBounded(t *testing.T) {
	b := Backoff{Base: 100 * time.Millisecond, Max: time.Second}
	// attempt 4 → Base*2^4 = 1600ms capped at Max=1s; rnd=1.0 → full window
	if got := b.For(4, 1.0); got != time.Second {
		t.Fatalf("capped delay = %v, want 1s", got)
	}
	// rnd=0 → 0
	if got := b.For(2, 0); got != 0 {
		t.Fatalf("rnd=0 delay = %v, want 0", got)
	}
	// rnd=0.5 attempt0 → Base*1*0.5 = 50ms
	if got := b.For(0, 0.5); got != 50*time.Millisecond {
		t.Fatalf("delay = %v, want 50ms", got)
	}
}

type flakySink struct {
	failFirst int
	calls     int
}

func (f *flakySink) Name() string { return "flaky" }
func (f *flakySink) EmitEvents(context.Context, []model.Event) error {
	f.calls++
	if f.calls <= f.failFirst {
		return errors.New("transient")
	}
	return nil
}

func TestDeliverRetriesThenSucceeds(t *testing.T) {
	s := &flakySink{failFirst: 2}
	var slept []time.Duration
	err := DeliverWithRetry(context.Background(), s, nil, 5,
		Backoff{Base: 10 * time.Millisecond, Max: time.Second},
		func(d time.Duration) { slept = append(slept, d) },
		func() float64 { return 0.0 },
	)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if s.calls != 3 {
		t.Fatalf("want 3 calls, got %d", s.calls)
	}
	if len(slept) != 2 {
		t.Fatalf("want 2 backoff sleeps, got %d", len(slept))
	}
}

func TestDeliverGivesUpAfterMaxAttempts(t *testing.T) {
	s := &flakySink{failFirst: 100}
	err := DeliverWithRetry(context.Background(), s, nil, 3,
		Backoff{Base: time.Millisecond, Max: time.Second},
		func(time.Duration) {}, func() float64 { return 0 },
	)
	if err == nil {
		t.Fatal("expected failure after max attempts")
	}
	if s.calls != 3 {
		t.Fatalf("want 3 attempts, got %d", s.calls)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/queue/ -v`
Expected: FAIL — `Backoff` / `DeliverWithRetry` undefined.

- [ ] **Step 3: 구현**

Create `internal/queue/queue.go`:
```go
// Package queue provides retry-with-backoff delivery to sinks. Full Jitter
// backoff and injected clock/random keep it deterministic under test.
package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/nodevitals/nodevitals/internal/model"
	"github.com/nodevitals/nodevitals/internal/sink"
)

// Backoff computes Full Jitter delays: random(0, min(Max, Base*2^attempt)).
type Backoff struct {
	Base time.Duration
	Max  time.Duration
}

// For returns the delay for a 0-indexed attempt. rnd is in [0,1].
func (b Backoff) For(attempt int, rnd float64) time.Duration {
	window := b.Base << attempt
	if window > b.Max || window <= 0 {
		window = b.Max
	}
	return time.Duration(rnd * float64(window))
}

// DeliverWithRetry emits events, retrying transient failures with backoff.
// sleep and rnd are injected for deterministic tests (use time.Sleep and
// rand.Float64 in production).
func DeliverWithRetry(
	ctx context.Context, s sink.Sink, events []model.Event, maxAttempts int,
	b Backoff, sleep func(time.Duration), rnd func() float64,
) error {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			sleep(b.For(attempt-1, rnd()))
		}
		lastErr = s.EmitEvents(ctx, events)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("sink %s failed after %d attempts: %w", s.Name(), maxAttempts, lastErr)
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/queue/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/queue/
git commit -m "feat(queue): full-jitter backoff retry delivery with injected clock"
```

---

### Task 9: HTTP API (REST 스냅샷 + /metrics 마운트)

**Files:**
- Create: `internal/httpapi/server.go`, `internal/httpapi/server_test.go`

**Interfaces:**
- Consumes: `model.Sample`, `sink.Metrics` (Handler())
- Produces: `httpapi.SnapshotSource` interface `{ Snapshot() []model.Sample }`, `httpapi.NewServer(src SnapshotSource, metricsHandler http.Handler) *http.ServeMux`. `GET /v1/state` → JSON `[]model.Sample`. `GET /metrics` → metricsHandler. `GET /healthz` → 200 "ok".

- [ ] **Step 1: 실패 테스트 작성**

Create `internal/httpapi/server_test.go`:
```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nodevitals/nodevitals/internal/model"
)

type stubSrc struct{ s []model.Sample }

func (s stubSrc) Snapshot() []model.Sample { return s.s }

func TestStateEndpointReturnsSnapshot(t *testing.T) {
	src := stubSrc{s: []model.Sample{{Node: "n", Metric: "load1", Value: 2}}}
	mux := NewServer(src, http.NotFoundHandler())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/state")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got []model.Sample
	json.NewDecoder(resp.Body).Decode(&got)
	if len(got) != 1 || got[0].Metric != "load1" {
		t.Fatalf("bad snapshot: %+v", got)
	}
}

func TestHealthzOK(t *testing.T) {
	mux := NewServer(stubSrc{}, http.NotFoundHandler())
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/healthz")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/httpapi/ -v`
Expected: FAIL — `NewServer` undefined.

- [ ] **Step 3: 구현**

Create `internal/httpapi/server.go`:
```go
// Package httpapi serves the REST snapshot, /metrics, and health endpoints.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/nodevitals/nodevitals/internal/model"
)

// SnapshotSource provides the current sample snapshot for GET /v1/state.
type SnapshotSource interface {
	Snapshot() []model.Sample
}

func NewServer(src SnapshotSource, metricsHandler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(src.Snapshot())
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /metrics", metricsHandler)
	return mux
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/httpapi/ -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/
git commit -m "feat(httpapi): REST snapshot, metrics mount, healthz"
```

---

### Task 10: Agent Core (파이프라인 조립 + 수명주기)

collector → event engine → sinks 를 tick 마다 조립. 최신 snapshot 보유(REST 소스).

**Files:**
- Create: `internal/agent/agent.go`, `internal/agent/agent_test.go`

**Interfaces:**
- Consumes: `collector.Registry`, `event.Engine`, `sink.Sink`(webhook), `sink.Metrics`, `queue.Backoff`/`DeliverWithRetry`, `config.Config`
- Produces: `agent.New(cfg config.Config, reg *collector.Registry, eng *event.Engine, webhooks []sink.Sink, metrics *sink.Metrics) *Agent`, `func (a *Agent) Tick(ctx context.Context)` (한 사이클: collect → metrics.Update → engine.Evaluate → 각 webhook 으로 DeliverWithRetry), `func (a *Agent) Snapshot() []model.Sample` (httpapi.SnapshotSource 구현).

- [ ] **Step 1: 실패 테스트 작성**

Create `internal/agent/agent_test.go`:
```go
package agent

import (
	"context"
	"testing"

	"github.com/nodevitals/nodevitals/internal/collector"
	"github.com/nodevitals/nodevitals/internal/config"
	"github.com/nodevitals/nodevitals/internal/event"
	"github.com/nodevitals/nodevitals/internal/model"
	"github.com/nodevitals/nodevitals/internal/sink"
)

type captureSink struct{ events []model.Event }

func (c *captureSink) Name() string { return "capture" }
func (c *captureSink) EmitEvents(_ context.Context, ev []model.Event) error {
	c.events = append(c.events, ev...)
	return nil
}

type fixedCollector struct{ v float64 }

func (f fixedCollector) Name() string { return "fixed" }
func (f fixedCollector) Collect(context.Context) ([]model.Sample, error) {
	return []model.Sample{{Node: "n", Tier: "core", Device: "cpu", Metric: "load1", Value: f.v}}, nil
}

func TestTickCollectsUpdatesMetricsAndDeliversEvents(t *testing.T) {
	cfg := config.Config{
		Node: "n", Tier: "core",
		Rules: []config.Rule{{Metric: "load1", Device: "cpu", Condition: "load_high", Severity: "warning", Threshold: 4, EnterFor: 1, ExitFor: 1}},
	}
	var reg collector.Registry
	reg.Add(fixedCollector{v: 9}) // above threshold → ENTER on first tick (EnterFor=1)
	eng := event.NewEngine("n", cfg.Rules)
	cap := &captureSink{}
	metrics := sink.NewMetrics()

	a := New(cfg, &reg, eng, []sink.Sink{cap}, metrics)
	a.Tick(context.Background())

	if len(cap.events) != 1 || cap.events[0].Phase != model.PhaseEnter {
		t.Fatalf("want 1 ENTER event delivered, got %+v", cap.events)
	}
	if snap := a.Snapshot(); len(snap) != 1 || snap[0].Value != 9 {
		t.Fatalf("snapshot not updated: %+v", snap)
	}
}

func TestTickNoEventWhenBelowThreshold(t *testing.T) {
	cfg := config.Config{Node: "n", Tier: "core",
		Rules: []config.Rule{{Metric: "load1", Device: "cpu", Condition: "load_high", Threshold: 4, EnterFor: 1, ExitFor: 1}}}
	var reg collector.Registry
	reg.Add(fixedCollector{v: 1})
	cap := &captureSink{}
	a := New(cfg, &reg, event.NewEngine("n", cfg.Rules), []sink.Sink{cap}, sink.NewMetrics())
	a.Tick(context.Background())
	if len(cap.events) != 0 {
		t.Fatalf("want no events, got %+v", cap.events)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/agent/ -v`
Expected: FAIL — `New` undefined.

- [ ] **Step 3: 구현**

Create `internal/agent/agent.go`:
```go
// Package agent wires collectors, the event engine, and sinks into a run loop.
package agent

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/nodevitals/nodevitals/internal/collector"
	"github.com/nodevitals/nodevitals/internal/config"
	"github.com/nodevitals/nodevitals/internal/event"
	"github.com/nodevitals/nodevitals/internal/model"
	"github.com/nodevitals/nodevitals/internal/queue"
	"github.com/nodevitals/nodevitals/internal/sink"
)

type Agent struct {
	cfg      config.Config
	reg      *collector.Registry
	eng      *event.Engine
	webhooks []sink.Sink
	metrics  *sink.Metrics
	backoff  queue.Backoff

	mu   sync.RWMutex
	snap []model.Sample
}

func New(cfg config.Config, reg *collector.Registry, eng *event.Engine, webhooks []sink.Sink, metrics *sink.Metrics) *Agent {
	return &Agent{
		cfg: cfg, reg: reg, eng: eng, webhooks: webhooks, metrics: metrics,
		backoff: queue.Backoff{Base: 500 * time.Millisecond, Max: 30 * time.Second},
	}
}

// Tick runs one collect→evaluate→deliver cycle.
func (a *Agent) Tick(ctx context.Context) {
	samples := a.reg.CollectAll(ctx)

	a.mu.Lock()
	a.snap = samples
	a.mu.Unlock()

	if a.metrics != nil {
		a.metrics.Update(samples)
	}

	events := a.eng.Evaluate(samples)
	if len(events) == 0 {
		return
	}
	for _, s := range a.webhooks {
		if err := queue.DeliverWithRetry(ctx, s, events, 5, a.backoff, time.Sleep, rand.Float64); err != nil {
			slog.Error("event delivery failed", "sink", s.Name(), "err", err)
		}
	}
}

// Snapshot implements httpapi.SnapshotSource.
func (a *Agent) Snapshot() []model.Sample {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.snap
}

// Run ticks on the configured interval until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	t := time.NewTicker(a.cfg.Interval())
	defer t.Stop()
	a.Tick(ctx) // immediate first tick
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.Tick(ctx)
		}
	}
}
```

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/agent/ -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/agent/
git commit -m "feat(agent): wire collectors, event engine, and sinks into tick loop"
```

---

### Task 11: main.go (entrypoint) + 빌드 스모크

**Files:**
- Create: `cmd/nodevitals/main.go`

**Interfaces:**
- Consumes: 전 패키지. loadavg 콜렉터 하나만 등록(core tier walking skeleton).

- [ ] **Step 1: 구현**

Create `cmd/nodevitals/main.go`:
```go
// Command nodevitals runs the hardware telemetry agent.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/nodevitals/nodevitals/internal/agent"
	"github.com/nodevitals/nodevitals/internal/collector"
	"github.com/nodevitals/nodevitals/internal/config"
	"github.com/nodevitals/nodevitals/internal/event"
	"github.com/nodevitals/nodevitals/internal/httpapi"
	"github.com/nodevitals/nodevitals/internal/sink"
)

func main() {
	cfgPath := flag.String("config", "/etc/nodevitals/config.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	if cfg.Node == "" {
		cfg.Node = os.Getenv("NODE_NAME") // downward API
	}

	var reg collector.Registry
	reg.Add(collector.NewLoadAvg(cfg.Node, cfg.ProcRoot))

	eng := event.NewEngine(cfg.Node, cfg.Rules)

	var webhooks []sink.Sink
	for _, w := range cfg.Sinks.Webhook {
		webhooks = append(webhooks, sink.NewWebhook(w, nil))
	}
	metrics := sink.NewMetrics()

	a := agent.New(cfg, &reg, eng, webhooks, metrics)

	mux := httpapi.NewServer(a, metrics.Handler())
	listen := cfg.Sinks.Metrics.ListenAddr
	if listen == "" {
		listen = ":9847"
	}
	srv := &http.Server{Addr: listen, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", "err", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	slog.Info("nodevitals started", "node", cfg.Node, "tier", cfg.Tier, "listen", listen)
	a.Run(ctx)

	_ = srv.Shutdown(context.Background())
}
```

- [ ] **Step 2: 전체 빌드 확인**

Run: `go build ./...`
Expected: 에러 없음, 종료 코드 0.

- [ ] **Step 3: 전체 테스트 확인**

Run: `go test ./...`
Expected: 전 패키지 PASS (ok model/config/collector/event/sink/queue/httpapi/agent).

- [ ] **Step 4: vet**

Run: `go vet ./...`
Expected: 출력 없음, 종료 코드 0.

- [ ] **Step 5: 실 바이너리 스모크 (fixture config 로 기동→헬스체크→종료)**

```bash
mkdir -p /tmp/nv && printf '0.10 0.20 0.30 1/1 1\n' > /tmp/nv/loadavg
cat > /tmp/nv/config.yaml <<'EOF'
node: smoke
tier: core
intervalSeconds: 1
procRoot: /tmp/nv
sinks:
  metrics:
    enabled: true
    listenAddr: ":9847"
rules:
  - metric: load1
    device: cpu
    condition: load_high
    severity: warning
    threshold: 0.05
    enterFor: 1
    exitFor: 1
EOF
go run ./cmd/nodevitals -config /tmp/nv/config.yaml &
NVPID=$!
sleep 2
curl -sf localhost:9847/healthz && echo " [healthz ok]"
curl -sf localhost:9847/metrics | grep nodevitals_hw_load1 && echo "[metrics ok]"
curl -sf localhost:9847/v1/state && echo " [state ok]"
kill $NVPID
```
Expected: `[healthz ok]`, `nodevitals_hw_load1{...} 0.1` 라인 + `[metrics ok]`, `[state ok]`.

- [ ] **Step 6: Commit**

```bash
git add cmd/
git commit -m "feat(cmd): nodevitals entrypoint wiring core tier walking skeleton"
```

---

### Task 12: Helm 차트 (core tier DaemonSet)

**Files:**
- Create: `deploy/chart/Chart.yaml`, `deploy/chart/values.yaml`, `deploy/chart/templates/_helpers.tpl`, `deploy/chart/templates/configmap.yaml`, `deploy/chart/templates/daemonset.yaml`

**Interfaces:**
- Consumes: 컨테이너 이미지 `ghcr.io/nodevitals/nodevitals`, config.yaml 스키마(Task 3)

- [ ] **Step 1: Chart.yaml**

Create `deploy/chart/Chart.yaml`:
```yaml
apiVersion: v2
name: nodevitals
description: Unified hardware telemetry agent for Kubernetes nodes
type: application
version: 0.1.0
appVersion: "0.1.0"
```

- [ ] **Step 2: values.yaml (tier 토글 구조 — core 만 v0.1 구현)**

Create `deploy/chart/values.yaml`:
```yaml
image:
  repository: ghcr.io/nodevitals/nodevitals
  tag: ""            # 기본은 Chart.appVersion
  pullPolicy: IfNotPresent

# tier 별 DaemonSet 렌더 토글. v0.1 은 core 만 구현.
tiers:
  core:
    enabled: true
  gpu:
    enabled: false   # 후속 마일스톤
  smart:
    enabled: false   # 후속 마일스톤

intervalSeconds: 15

# 이벤트 룰 (기본 예시 — 사용자 override)
rules:
  - metric: load1
    device: cpu
    condition: load_high
    severity: warning
    threshold: 8.0
    enterFor: 3
    exitFor: 3

# 고객 백엔드 webhook (Secret 참조 권장 — 여기선 최소)
webhooks: []
#  - url: https://backend.example/hook
#    secret: ""

metrics:
  listenAddr: ":9847"

resources:
  requests:
    cpu: 20m
    memory: 32Mi
  limits:
    memory: 64Mi
```

- [ ] **Step 3: _helpers.tpl**

Create `deploy/chart/templates/_helpers.tpl`:
```
{{- define "nodevitals.name" -}}nodevitals{{- end -}}
{{- define "nodevitals.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{ .Values.image.repository }}:{{ $tag }}
{{- end -}}
```

- [ ] **Step 4: configmap.yaml (core tier config 렌더)**

Create `deploy/chart/templates/configmap.yaml`:
```yaml
{{- if .Values.tiers.core.enabled }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: nodevitals-core
  labels:
    app.kubernetes.io/name: nodevitals
    app.kubernetes.io/component: core
data:
  config.yaml: |
    tier: core
    intervalSeconds: {{ .Values.intervalSeconds }}
    procRoot: /host/proc
    rules:
{{ toYaml .Values.rules | indent 6 }}
    sinks:
      metrics:
        enabled: true
        listenAddr: "{{ .Values.metrics.listenAddr }}"
      webhook:
{{ toYaml .Values.webhooks | indent 8 }}
{{- end }}
```

- [ ] **Step 5: daemonset.yaml (core tier — 무특권, read-only /proc hostPath)**

Create `deploy/chart/templates/daemonset.yaml`:
```yaml
{{- if .Values.tiers.core.enabled }}
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: nodevitals-core
  labels:
    app.kubernetes.io/name: nodevitals
    app.kubernetes.io/component: core
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: nodevitals
      app.kubernetes.io/component: core
  template:
    metadata:
      labels:
        app.kubernetes.io/name: nodevitals
        app.kubernetes.io/component: core
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: nodevitals
          image: {{ include "nodevitals.image" . | quote }}
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          args: ["-config", "/etc/nodevitals/config.yaml"]
          env:
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
          ports:
            - name: metrics
              containerPort: 9847
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
          volumeMounts:
            - name: proc
              mountPath: /host/proc
              readOnly: true
            - name: config
              mountPath: /etc/nodevitals
              readOnly: true
          resources:
{{ toYaml .Values.resources | indent 12 }}
          livenessProbe:
            httpGet:
              path: /healthz
              port: metrics
      volumes:
        - name: proc
          hostPath:
            path: /proc
        - name: config
          configMap:
            name: nodevitals-core
{{- end }}
```

- [ ] **Step 6: helm 렌더 검증 (core 만 렌더, gpu/smart 는 렌더 안 됨)**

Run:
```bash
helm template nv deploy/chart | grep -c "kind: DaemonSet"
helm template nv deploy/chart --set tiers.gpu.enabled=true 2>&1 | grep -c "component: gpu" || true
```
Expected: 첫 명령 = `1` (core DaemonSet 1개). 두 번째 = `0` (gpu 템플릿 미구현이라 렌더 없음 — v0.1 정상).

- [ ] **Step 7: kubeconform 검증 (설치 없이 스키마)**

Run:
```bash
helm template nv deploy/chart | kubeconform -strict -summary
```
Expected: `Valid` 리소스만, invalid 0. (kubeconform 미설치 시 `go install github.com/yannh/kubeconform/cmd/kubeconform@latest` 후 재실행. 완전 미가용 시 `helm template nv deploy/chart | grep -E "kind:|name:"` 로 수기 확인 + 이 스텝 skip 사유 커밋 본문 명시)

- [ ] **Step 8: Commit**

```bash
git add deploy/
git commit -m "feat(chart): Helm chart rendering core-tier DaemonSet (unprivileged)"
```

---

### Task 13: Makefile (로컬 게이트) + 최종 회귀

**Files:**
- Create: `Makefile`

**Interfaces:** (없음 — 로컬 게이트 집약)

- [ ] **Step 1: Makefile**

Create `Makefile`:
```makefile
.PHONY: test vet build docker chart-lint all

test:
	go test ./...

vet:
	go vet ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/nodevitals ./cmd/nodevitals

docker:
	docker build -t ghcr.io/nodevitals/nodevitals:dev .

chart-lint:
	helm template nv deploy/chart | kubeconform -strict -summary

all: vet test build
```

- [ ] **Step 2: 로컬 게이트 전체 실행**

Run: `make all`
Expected: vet 무출력 → test 전 패키지 PASS → `dist/nodevitals` 생성.

- [ ] **Step 3: 이미지 빌드 스모크 (Docker 가용 시)**

Run: `make docker`
Expected: 빌드 성공, `ghcr.io/nodevitals/nodevitals:dev` 이미지 생성. (Docker 미가용 시 skip + 커밋 본문에 사유 명시.)

- [ ] **Step 4: 최종 회귀 — 전체 재확인**

Run:
```bash
go build ./... && go vet ./... && go test ./... && helm template nv deploy/chart >/dev/null && echo "ALL GREEN"
```
Expected: `ALL GREEN`.

- [ ] **Step 5: Commit**

```bash
git add Makefile
git commit -m "chore: Makefile local gates (vet/test/build/docker/chart)"
```

---

## Self-Review (작성자 체크 — 완료)

**1. Spec coverage (스펙 §별 → Task 매핑):**
- §4 Tiered Single-Agent → Task 12 (chart tier 토글, core 구현 / gpu·smart 스텁). GPU·SMART tier *구현* 은 후속 마일스톤(명시적 범위 밖).
- §5 컴포넌트 → collector(T4)/event(T5)/sink(T6,T7)/queue(T8)/agent(T10)/config(T3)/httpapi(T9) 전부 매핑.
- §6 데이터 모델 → T2(Sample/Event), CloudEvents 엔벨로프 → T6.
- §7 전달 3표면 → webhook(T6)/metrics(T7)/REST(T9). event webhook = webhook sink 가 이벤트 운반(T6+T10).
- §8 신뢰성 → T8(Full Jitter 백오프·재시도). 유계 융합큐·치명레인·드롭회계·위상오프셋 = **후속**(walking skeleton 은 재시도까지. 스펙 §8 전체는 M2 배송 강화에서 완성 — 아래 갭 명시).
- §9 수집 → core tier 대표로 loadavg 1종(T4). cpu-util/mem/disk/net/hwmon/gpu/smart = 후속(패턴 확립됨).
- §10 이벤트 엔진 → T5(상태전이+히스테리시스). HW별 condition 목록은 콜렉터 추가 시 룰로.
- §11 보안 → T12 core tier(무특권/drop ALL/readOnlyRootFS). smart 특권 tier = 후속.
- §12 패키징 → T1(Dockerfile distroless/static)/T12(chart)/T13(Makefile). 멀티아치 매니페스트 발행 = 후속 릴리스.
- §13 테스트 → 전 Task fixture/mock/httptest, 하드웨어 0대. ✓
- §14 v0.1 = 본 M1 은 그중 **walking skeleton**(foundation + core 수직 슬라이스). GPU/SMART/추가콜렉터/배송강화 = 후속 계획.

**갭 (의도적 — M1 밖, 후속 계획):** GPU tier(NVML+XID), SMART tier(anatol/smart.go+특권), 추가 core 콜렉터(cpu-util/mem/disk/net/hwmon), 유계 융합큐·치명레인·드롭회계·위상오프셋(§8 완성), 멀티아치 이미지 발행, CI(ADR 필요). → **각각 별도 마일스톤 계획으로 작성 예정.**

**2. Placeholder scan:** TBD/TODO/"적절히"/"등" 없음. 전 코드 스텁 완전. 유일한 조건부 = kubeconform/docker 미설치 시 skip+사유(정직한 환경 분기, 플레이스홀더 아님).

**3. Type consistency:** `Collector.Collect(ctx)([]model.Sample,error)`, `Sink.EmitEvents(ctx,[]model.Event)error`, `event.NewEngine/Evaluate`, `sink.NewWebhook/NewMetrics`, `queue.Backoff.For/DeliverWithRetry`, `agent.New/Tick/Snapshot`, `httpapi.NewServer/SnapshotSource` — Task 간 시그니처 일치 확인. `model.Sample`/`model.Event` 필드명 전 Task 일관.
