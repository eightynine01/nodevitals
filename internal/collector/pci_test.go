package collector

import "testing"

// TestInt8ToString pins the PciInfo.BusId [32]int8 → Go string conversion:
// copy bytes until the first NUL. NVML hands back a fixed-width C char array
// with the real bus id NUL-terminated and the rest zero-padded, so the helper
// must stop at the NUL and never leak the trailing zeros into the label.
func TestInt8ToString(t *testing.T) {
	// NUL-terminated: real PCI bus id followed by zero padding (the [32]int8
	// shape NVML returns) → padding stripped, exact string back.
	busID := "00000000:65:00.0"
	buf := make([]int8, 32)
	for i, c := range []byte(busID) {
		buf[i] = int8(c)
	}
	if got := int8ToString(buf); got != busID {
		t.Fatalf("int8ToString(NUL-terminated) = %q, want %q", got, busID)
	}

	// No NUL anywhere: every slot is a non-zero byte → the whole array back.
	full := []int8{'a', 'b', 'c'}
	if got := int8ToString(full); got != "abc" {
		t.Fatalf("int8ToString(no NUL) = %q, want %q", got, "abc")
	}

	// All-zero (first byte is NUL) and empty both yield "".
	if got := int8ToString(make([]int8, 16)); got != "" {
		t.Fatalf("int8ToString(all-zero) = %q, want empty", got)
	}
	if got := int8ToString(nil); got != "" {
		t.Fatalf("int8ToString(nil) = %q, want empty", got)
	}
}
