package buildinfo

import "testing"

func TestFormat(t *testing.T) {
	for _, test := range []struct{ version, commit, want string }{
		{"dev", "unknown", "dev (commit unknown)"},
		{"v1.2.3", "2026-11-20T00:00:00Z", "v1.2.3 (commit 2026-11-20T00:00:00Z)"},
		{"dev", "", "dev (commit unknown)"},
	} {
		if got := Format(test.version, test.commit); got != test.want {
			t.Errorf("Format(%q, %q) = %q, want %q", test.version, test.commit, got, test.want)
		}
	}
}
