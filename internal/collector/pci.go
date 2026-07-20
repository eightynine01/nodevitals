package collector

// int8ToString converts a NUL-terminated C char array (as int8, which is what
// C `char` is on linux/amd64) into a Go string, stopping at the first NUL and
// dropping the trailing zero padding. It lives here — untagged, pure Go —
// rather than in gpu_nvml.go so it stays unit-testable under CGO_ENABLED=0,
// the same pattern throttle.go/xid.go use to keep decode logic off the
// cgo-bound side of the gpuReader seam. The caller (gpu_nvml.go, gpu-tagged)
// slices nvml.PciInfo.BusId ([32]int8) at the cgo boundary and hands the slice
// here; the conversion itself is plain Go.
func int8ToString(cs []int8) string {
	b := make([]byte, 0, len(cs))
	for _, c := range cs {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}
