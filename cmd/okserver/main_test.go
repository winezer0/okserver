package main

import (
	"testing"
)

func TestVersionFlagsExitSuccessfullyBeforeConfig(t *testing.T) {
	for _, arg := range []string{"--version", "-v"} {
		if !run([]string{arg}) {
			t.Fatalf("%s did not exit through version path", arg)
		}
	}
}
