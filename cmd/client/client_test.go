package client

import "testing"

func TestRemovedGzipFlagIsRejected(t *testing.T) {
	if err := Run("okserver", []string{"-g"}); err == nil {
		t.Fatal("removed gzip flag accepted")
	}
}
