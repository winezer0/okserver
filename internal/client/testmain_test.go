package client

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "okserver-client-cache-")
	if err != nil {
		panic(err)
	}
	os.Setenv("OKSERVER_CACHE_DIR", directory)
	code := m.Run()
	os.RemoveAll(directory)
	os.Exit(code)
}
