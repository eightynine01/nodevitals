# Branding Guide — `nodevitals`

> Visual identity, voice, and tone for the `nodevitals` agent.

This document is the canonical reference for `nodevitals` branding decisions.
It applies to the README, release notes, and any external communication about
the project.

## 1. Identity

**Organization**: [keiailab](https://keiailab.com).

**Project**: `nodevitals` — a unified hardware telemetry agent for Kubernetes
nodes (CPU, memory, disk/SMART, NIC, sensors, NVIDIA GPU), shipped as a
DaemonSet and a Helm chart. It is not an operator: it defines no custom
resources and reconciles none.

## 2. Logo and visual assets

| Asset | URL | Usage |
|---|---|---|
| Current primary logo | `docs/branding/symbol.png` | README header, Artifact Hub icon/screenshot |
| Keiailab base symbol | `docs/branding/base-symbol.png` | Source reference for the outer rotating-arrow mark |
| Current favicon | `https://keiailab.com/favicon.ico` | Favicon, social cards |

**Logo placement**: Top-center of README, width 96 px. Always link to
`https://keiailab.com`.

**Clear space**: Minimum padding around the logo equals 25 % of the logo width.

**Do not**:

- Recolor the logo
- Add drop shadows or filters
- Place the logo on backgrounds with insufficient contrast
- Combine with other logos without keiailab brand approval

> `docs/branding/symbol.png` is currently a byte-identical copy of
> `base-symbol.png` — the shared keiailab mark with the plain gradient sphere
> at its centre. When a nodevitals service icon is drawn, replace the centre
> sphere only, keep the outer rotating-arrow ring, and overwrite `symbol.png`
> in place. Every reference (README, chart `icon`, Artifact Hub screenshot)
> points at that one path, so no other file changes.

## 3. Color palette

| Role | Hex | Usage |
|---|---|---|
| Primary (keiailab teal) | `#0EA5A8` | Headers, primary actions, links |
| Secondary (deep navy) | `#0F172A` | Dark backgrounds, code blocks |
| Accent (warm amber) | `#F59E0B` | Highlights, badge accents |
| Neutral grey | `#64748B` | Body text on light backgrounds |
| Background light | `#F8FAFC` | Documentation page background |
| Background dark | `#020617` | Dark-mode code editor theme |

## 4. Typography

- **Headings**: System default (GitHub default: `-apple-system, BlinkMacSystemFont, Segoe UI, ...`)
- **Body**: System default (GitHub-native consistency)
- **Code**: `ui-monospace, SFMono-Regular, Consolas, ...` (GitHub default monospace)

No external web font is used — keep rendering identical to native GitHub.

## 5. Voice and tone

**Audience**: Kubernetes platform engineers, SREs, and hardware/observability
owners.

**Voice principles**:

- **Direct** — prefer bullet points over paragraphs where possible.
- **Evidence-based** — claims include a measurement, a benchmark, or a link.
- **Agent-focused** — `nodevitals` collects and delivers telemetry. Alerting,
  dashboards, and paging belong to the consumer (Prometheus / Alertmanager, or
  the webhook backend), not to this agent.

**Avoid**:

- Marketing superlatives ("blazing fast", "revolutionary", "best-in-class").
- Vague comparisons ("enterprise-grade quality") — qualify each claim with a
  specific metric.
- Time-based deadlines in the roadmap — use a feature checklist.

## 6. README header standard

Every README's first block follows this layout:

```markdown
<p align="center">
  <a href="https://keiailab.com">
    <img src="docs/branding/symbol.png" alt="keiailab" width="96"/>
  </a>
</p>

# nodevitals

**Unified hardware telemetry for Kubernetes nodes — one agent instead of three exporters.**

<!-- badge lines, see § 7 -->

## Design assets

<!-- Asset | Path | Usage table -->
```

## 7. Badge order

The shields.io badges in the README appear in this order (left → right), all as
static `img.shields.io/badge/...` shields except Release, which must reflect
live repository state:

1. License (MIT) → `LICENSE`
2. Go version → `go.mod`
3. Kubernetes
4. Release
