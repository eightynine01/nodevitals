# nodevitals ↔ Kakao Cloud AI Insight — 갭 분석 및 축소 백로그

> **현재 진행 증분: G1/G3/G4 (GPU 텔레메트리 완결성)** — 구현 완료(로컬 게이트 green + gpu-tagged Docker 컴파일 검증).
> **구현 결정(적대검증 반영)**: G1=UUID/모델/PCI 라벨을 `Sample.Labels`로 방출 + sink 승격(`sort` 결정론으로 descriptor 일관성). G3=`gpu_ecc_corrected_total`(SBE polled counter) 추가 — *전용 SBE/DBE 이벤트 구독은 순손해로 평가·폐기*(watchXid가 그 이벤트를 소비 안 함 + all-or-nothing RegisterEvents의 XID 회귀 위험 + SBE storm 오버헤드; ECC 신호는 polled 카운터+XID 분류(48/92/94/95)가 이미 보유). G4=`gpu_throttle_active{reason}` — *성능저하 유발 비트만* 방출(benign `gpu_idle`/`app_clocks`/`sync_boost`/`display` 제외 → 유휴 GPU 오탐 방지), raw `gpu_throttle_reasons` mask는 무손실 유지.
> **상태** Draft · 2026-07-20 · **repo** <https://github.com/KeiaiLab/nodevitals>
> **비교 대상** Kakao Cloud AI Insight (<https://docs.kakaocloud.com/service/ai-service/ai-insight/ai-insight-overview>)
> 코드 식별자·스키마·플래그는 영어, 서술은 한국어. 본 문서는 코드 실측(v0.2-dev, HEAD `e2910a6`)에 근거한다.

---

## 0. 한눈에 — 두 제품은 레이어가 다르다

| | nodevitals | Kakao Cloud AI Insight |
|---|---|---|
| **정체성** | 노드 하드웨어 텔레메트리 **수집·전달 에이전트** (단일 Go 바이너리) | GPU 관측·관리 **플랫폼** (UI + 상태 분류 + 집계) |
| **범위** | collect → event engine → webhook/REST/`/metrics` | Overview·GPU Map·GPU Explorer·이벤트 분석 UI |
| **커버리지** | CPU·mem·disk/SMART·NIC·sensors **+ GPU** (하드웨어 전방위) | **GPU 중심** + 노드 시스템 지표 |
| **명시적 비목표** | "대시보드가 아니다 — Grafana/백엔드는 사용자 몫" (README) | 시각화·집계가 핵심 가치 |
| **AI/LLM** | 없음 (규칙 기반 임계치 + 히스테리시스) | 없음 (규칙 기반 임계치 + 상관 탐색) — 이름과 달리 LLM 미사용 |

**핵심 관찰**: AI Insight의 "AI"는 LLM이 아니라 **규칙 기반 상태 분류 + 상관 탐색 UI**다. 따라서 갭은 "지능"이 아니라 **① GPU 데이터 모델의 완결성**(정체성·MIG·ECC·throttle 의미론)과 **② 상태 롤업/집계 차원**에 있다. UI 계층(Map/Explorer/Overview)은 nodevitals가 **자기 선언으로 비목표** 삼은 영역이므로 갭이 아니라 **소비 백엔드의 책임**이다 (§3에서 명시 분리).

---

## 1. AI Insight 기능 → nodevitals 현황 매핑 (실측)

| AI Insight 기능 | nodevitals 현황 (코드 실측) | 판정 |
|---|---|---|
| 전체 GPU 현황 (총 GPU·클러스터·노드 수, 평균 사용률/메모리/온도, ECC 에러 수) | 노드별 raw 메트릭은 냄. **클러스터/노드 집계·평균은 백엔드 몫** | UI/집계 = 비목표 · 입력 라벨 부족(→G6) |
| GPU 상태 6분류 (Active/Idle/Warning/Critical/Pending/Agent Missing) | 이벤트 엔진은 **지표별 임계 전이**(ENTER/EXIT)만 생성 — **GPU 단위 복합 상태 없음** (`event.go`). idle/Active/Pending 개념 부재 | **갭 (G5)** |
| GPU Map (GPU/클러스터/노드별 시각화·상태 탐색) | 없음 — 시각화 계층 | 비목표 (백엔드) |
| GPU Explorer (계층별 상세 메트릭·이벤트) | polled 메트릭 7종 + XID 이벤트를 냄. **단, GPU 식별자(UUID/모델) 유실** → 탐색 대상을 특정 불가 | **갭 (G1)** |
| GPU 이벤트 분석 (ECC / XID / throttling / overheat) | XID 분류표(`xid.go`) 우수. **SBE ECC·throttle 이벤트 미배선** | **부분 갭 (G3, G4)** |
| MIG 인스턴스 확인 (인스턴스별 사용률·상태) | **전무** — `DeviceGetHandleByIndex`(물리 GPU)만 열거, MIG API 미사용 | **갭 (G2)** |
| 노드 시스템 지표 (CPU/mem/disk/network) | **core tier로 이미 커버** (node_exporter 대체) | **충족** ✅ |
| KE(K8s) + VM 양쪽 지원 | K8s DaemonSet(Helm) + `nodevitals -config` 단독 바이너리(=VM/베어메탈) | **충족** ✅ |
| Agent Missing 탐지 (익스포터 중단 감지) | scrape 경로는 Prometheus `up`이 자연 커버. **push 경로 heartbeat 없음** | **부분 갭 (G7)** |

---

## 2. 갭 축소 백로그 (우선순위 · 검증된 근거)

각 항목은 **AI Insight의 특정 기능**과 **수정할 파일**에 매핑된다. 우선순위는 "정체성 → GPU 관측 완결 → 롤업 → 검증" 순.

### P0 — 정체성 (이후 모든 항목의 전제)

**G1. GPU 식별 라벨 방출** — `Sample.Labels`에 UUID·모델·PCI·(MIG) 부착
- **근거**: `gpu_nvml.go:136-138`이 NVML에서 `UUID`·`Name`을 읽지만, `gpu.go:86-97`의 `Collect()`가 `mk()`로 샘플을 만들 때 `Labels`를 **비운 채** `Device: "gpu%d"`(로컬 인덱스)만 붙인다. 인덱스는 재부팅/재삽입 시 불안정 → 클러스터 간 GPU 추적 불가.
- **작업**: `gpuDevice`에 `PCIBusID`(`dev.GetPciInfo`) 추가 → `Collect()`에서 `Labels: {"uuid":d.UUID, "model":d.Name, "pci":d.PCIBusID}` 부착. `model.Sample.Labels`는 이미 존재(스키마 변경 0). 메트릭 sink가 라벨을 Prometheus label로 승격하는지 `internal/sink/metrics.go` 확인·보강.
- **닫는 기능**: GPU Explorer의 "특정 GPU 지목" 전제, 전체 현황의 GPU 카운트 정확도.

### P1 — GPU 관측 완결성

**G2. MIG 인스턴스 열거 + 인스턴스별 메트릭**
- **근거**: `internal/`·설계 문서 전체에 `mig` 문자열 0건. `gpu_nvml.go`는 물리 GPU만 열거.
- **작업**: `nvmlReader.Read()`에서 `dev.GetMigMode()` 확인 → 활성 시 `GetMaxMigDeviceCount`/`GetMigDeviceHandleByIndex` 순회, GI/CI id·프로파일 슬라이스로 `gpuDevice`에 `MigInstances []migInstance` 추가. 샘플 Device=`gpu<idx>/mig<gi>.<ci>`, Labels에 `gi_id`·`ci_id`·`profile`. fake `gpuReader`로 CGO=0 단위테스트, gpu-tagged 경로는 Docker 컴파일 체크(설계 §6 3-tier 검증 준수).
- **닫는 기능**: "MIG 인스턴스 확인" 전체.

**G3. SBE(단일비트) ECC 메트릭 + ECC 이벤트 구독** — 설계와 구현의 드리프트 교정
- **근거**: M2b 설계 §3은 `EventTypeXidCriticalError|DoubleBitEccError|SingleBitEccError` 구독을 명시하나, `gpu_nvml.go:68`은 **`EventTypeXidCriticalError`만** 등록. 메트릭도 `gpu_nvml.go:160`이 `MEMORY_ERROR_TYPE_UNCORRECTED`(DBE)만 읽어 `gpu_ecc_uncorrected_total` 하나뿐 → **SBE 카운터·이벤트 전무**.
- **작업**: (a) `GetTotalEccErrors(MEMORY_ERROR_TYPE_CORRECTED, AGGREGATE_ECC)` → `gpu_ecc_corrected_total`(KindCounter) 추가. (b) `RegisterEvents`에 SBE/DBE 비트 OR. (c) XID 분류표(`xid.go`)는 이미 SBE(92 warning)/DBE(48 critical) 구분 완비 — 유지.
- **닫는 기능**: AI Insight ECC 이벤트 분석의 SBE→Warning / DBE→Critical 구분, 전체 현황의 "ECC 에러 수"(정확히는 SBE+DBE 분리).

**G4. Throttle 디코딩 배선 + thermal/power throttle 이벤트** — 이미 만든 `decodeThrottle` 활용
- **근거**: `throttle.go`의 `decodeThrottle()`가 thermal(`sw/hw_thermal_slowdown`)·power(`sw_power_cap`·`hw_power_brake_slowdown`) 비트를 라벨로 디코딩하지만 **어디서도 호출 안 함**. `gpu.go:95`는 raw 비트마스크 `gpu_throttle_reasons`(float64)만 gauge로 방출.
- **작업**: `Collect()`에서 `decodeThrottle(d.ThrottleReasons)` 호출 → 이유별 `gpu_throttle_active{reason="hw_thermal_slowdown"}=1` 라벨 샘플 방출. 심각 비트(thermal/power)에 대한 내장 이벤트 규칙 또는 `config.Rule` 예시 추가. (설계 §4가 "throttle→이벤트는 후속"으로 남긴 항목 — 이제 배선.)
- **닫는 기능**: throttling 이벤트 분석(thermal vs power 구분), overheat 이벤트.

### P2 — 롤업 / 집계 의미론 (플랫폼이 기대하는 입력)

**G5. GPU 복합 상태 + idle 탐지** — AI Insight 6분류를 데이터 계층에서
- **근거**: `event.go`는 지표별 임계 전이만. GPU 단위로 "Active/Idle/Warning/Critical" 롤업 없음(grep `idle`→CPU·throttle 비트뿐, `Active`/`Pending` 상태 개념 부재).
- **작업**: 파생 샘플 `gpu_state`(enum: active/idle/warning/critical) 방출 — util 하한(idle) + temp/ecc/throttle(warning/critical) fold. 선택적으로 창(window) 기반 `gpu_idle_ratio`(유휴율). **Pending·Agent Missing 2개 상태는 노드 라이프사이클/데이터 부재 감지라 소비 백엔드 책임임을 문서에 명시** (nodevitals는 "이 GPU가 지금 무엇을 보고하는가"만 안다).
- **닫는 기능**: GPU 상태 6분류 중 에이전트가 알 수 있는 4개, 유휴율.

**G6. 토폴로지 라벨** — cluster/region/pool 정적 라벨 설정
- **근거**: `internal/`에 `cluster` 문자열 0건. 샘플은 `Node`만 있고 클러스터 차원 없음 → 백엔드가 클러스터 단위 집계 불가.
- **작업**: `config.go`에 `labels: {cluster: ..., region: ...}` 정적 맵 → 전 샘플·이벤트에 병합. Helm values로 주입. (nodevitals는 per-node라 클러스터 정체성은 설정에서 와야 함.)
- **닫는 기능**: 전체 GPU 현황의 클러스터/노드별 집계 **입력**(집계 UI 자체는 비목표).

**G7. Liveness heartbeat** — push 경로 "Agent Missing" 탐지 지원
- **근거**: `collector.go:35` 주석 "liveness는 agent-level self-metrics 사용". scrape는 `up`이 커버하나 webhook push는 침묵=정상과 구분 불가.
- **작업**: 주기적 `nodevitals_up`(gauge=1) + build-info 메트릭 + 선택적 heartbeat 이벤트(옵트인). 소비자가 부재 감지로 "Agent Missing" 판정.
- **닫는 기능**: Agent Missing 상태의 **탐지 입력**.

### P3 — 검증 (하드웨어 신뢰 게이트)

**G8. 실 GPU 스모크 테스트** — 이미 설계가 "ship 전 필수"로 이연한 항목
- **근거**: M2b 설계 §0 IMPORTANT + §8 리스크 — 개발 머신에 NVIDIA GPU 없음. NVML 런타임·`EventSetWait`·driver-reset은 실 GPU만 검증 가능.
- **작업**: GPU CI 러너 또는 사용자 GPU에서 `:v-gpu` 이미지 스모크 — G1~G5의 NVML 경로(UUID·MIG·ECC·throttle 실값)를 실측 확인. **G1~G5는 로직상 fake로 완전 단위테스트 가능하나, 하드웨어 값 정확성은 본 게이트가 유일 검증.**

---

## 3. 명시적 비목표 (갭이 아님 — nodevitals 설계 경계)

아래는 AI Insight엔 있으나 nodevitals가 **의도적으로 안 하는** 것 — "gap 축소" 대상에서 제외한다 (principles §2 단순성, README "대시보드 아님"):

- **GPU Map / GPU Explorer / Overview UI** — 시각화 계층. Grafana/자체 백엔드가 nodevitals의 `/metrics`·webhook·REST를 소비해 구현.
- **클러스터/노드 다층 집계·평균 대시보드** — 시계열 DB/백엔드의 집계 쿼리. nodevitals는 per-node raw + 라벨(G1/G6)만 제공.
- **Pending(노드 라이프사이클) 상태** — K8s/클라우드 노드 상태. 소비자가 노드 컨디션과 상관.

> 이 경계를 지키는 것이 갭 축소의 일부다: nodevitals는 **AI Insight 같은 소비자에게 완결된 데이터 소스**가 되는 것이 목표이지, AI Insight를 재구현하는 것이 아니다. G1·G3·G4·G6는 정확히 그 "완결된 입력"을 채운다.

---

## 4. 요약 — 우선순위 순 실행 큐

| ID | 작업 | 파일 | 닫는 AI Insight 기능 | 규모 |
|---|---|---|---|---|
| G1 | GPU 식별 라벨(UUID/모델/PCI) | `gpu.go`·`gpu_nvml.go`·`sink/metrics.go` | Explorer 지목·현황 정확도 | S |
| G2 | MIG 인스턴스 열거 | `gpu_nvml.go`·`gpu.go` | MIG 인스턴스 확인 | M |
| G3 | SBE ECC 메트릭+이벤트 구독 | `gpu_nvml.go` | ECC SBE/DBE 이벤트 분석 | S |
| G4 | throttle 디코딩+thermal/power 이벤트 | `gpu.go`·`throttle.go`·`config` | throttling/overheat 분석 | S |
| G5 | GPU 복합 상태+idle | `event.go` 또는 파생 콜렉터 | GPU 상태 분류(4/6)·유휴율 | M |
| G6 | 토폴로지 라벨(cluster 등) | `config.go`·차트 values | 클러스터 집계 입력 | S |
| G7 | liveness heartbeat | `agent.go`·`sink` | Agent Missing 탐지 입력 | S |
| G8 | 실 GPU 스모크 | CI/사용자 GPU | G1~G5 하드웨어 검증 게이트 | M |

**권장 진입**: G1(정체성) → G3+G4(이미 절반 구현된 저비용 완결) → G2(MIG) → G6+G7(집계·탐지 입력) → G5(롤업) → G8(하드웨어 검증). G3·G4는 설계-구현 드리프트/미배선 교정이라 **가장 적은 코드로 가장 큰 이벤트 분석 갭**을 닫는다.
