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
