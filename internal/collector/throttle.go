package collector

// throttleBits fixes clocks-throttle-reason decode order — map iteration is
// nondeterministic and would make decodeThrottle's output flaky, the same
// reason sataAttrMetrics (smart.go) is an ordered slice rather than a map.
// Bit values are NVML's nvmlClocksThrottleReasons* mask (nvml.h), ascending.
//
// Design doc §4 names 8 of these 9 bits explicitly (grouping benign vs.
// warning for a *future* throttle→event rule) and doesn't mention 0x8. That
// bit (HwSlowdown) is nonetheless a real, documented NVML throttle-reason
// bit, and v0.1 only exposes the raw decode as a gauge (no bit is dropped
// from decoding) — so it's included here for a complete decode of any mask
// NVML can actually report.
var throttleBits = []struct {
	mask  uint64
	label string
}{
	{0x1, "gpu_idle"},
	{0x2, "app_clocks_setting"},
	{0x4, "sw_power_cap"},
	{0x8, "hw_slowdown"},
	{0x10, "sync_boost"},
	{0x20, "sw_thermal_slowdown"},
	{0x40, "hw_thermal_slowdown"},
	{0x80, "hw_power_brake_slowdown"},
	{0x100, "display_clock_setting"},
}

// decodeThrottle decodes an NVML clocks-throttle-reasons bitmask
// (nvmlDeviceGetCurrentClocksThrottleReasons) into its set reason labels, in
// fixed ascending-bit order. Bits outside throttleBits are ignored. A zero
// mask yields an empty (non-nil) slice.
func decodeThrottle(mask uint64) []string {
	out := []string{}
	for _, b := range throttleBits {
		if mask&b.mask != 0 {
			out = append(out, b.label)
		}
	}
	return out
}

// benignThrottleReasons are clock-reason bits that do NOT indicate a
// performance-limiting throttle: idle (NVML sets gpu_idle whenever the GPU is
// not busy), application/user-set clocks, the sync-boost group, and the display
// clock floor. gpu_throttle_active (gpu.go) excludes these so an unfiltered
// `gpu_throttle_active == 1` alert does not false-positive on every idle GPU.
// The raw gpu_throttle_reasons mask still exposes every bit losslessly.
var benignThrottleReasons = map[string]bool{
	"gpu_idle":              true,
	"app_clocks_setting":    true,
	"sync_boost":            true,
	"display_clock_setting": true,
}
