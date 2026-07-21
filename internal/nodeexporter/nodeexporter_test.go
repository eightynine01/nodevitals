package nodeexporter

import "testing"

func TestValidateExtraFlagsRejectsPathOverrides(t *testing.T) {
	// extraFlags arrives from a ConfigMap. A --path.* entry appended after the
	// host paths would win and silently repoint every collector at this
	// container, so the numbers would describe the pod instead of the node.
	for _, f := range []string{
		"--path.procfs=/proc",
		"--path.rootfs=/",
		"--web.listen-address=:9999",
		"-collector.cpu",
		"",
	} {
		if err := validateExtraFlags([]string{f}); err == nil {
			t.Fatalf("validateExtraFlags(%q) allowed a non-collector flag", f)
		}
	}
}

func TestValidateExtraFlagsAllowsCollectorToggles(t *testing.T) {
	// Turning individual collectors on and off is the whole point of the field.
	ok := []string{"--collector.textfile.directory=/x", "--no-collector.zfs", "--collector.systemd"}
	if err := validateExtraFlags(ok); err != nil {
		t.Fatalf("validateExtraFlags(%v) rejected legitimate collector flags: %v", ok, err)
	}
	if err := validateExtraFlags(nil); err != nil {
		t.Fatalf("empty extraFlags must be fine, got %v", err)
	}
}
