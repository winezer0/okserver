package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadHashHeadersAndManifest(t *testing.T) {
	ts, root := newTestHTTPServer(t, "")
	response, err := http.Post(ts.URL+"/upload/abc.txt", "application/octet-stream", strings.NewReader("abc"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	response, err = ts.Client().Head(ts.URL + "/download/abc.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("X-File-SHA256") != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" || response.Header.Get("X-File-Size") != "3" {
		t.Fatal("invalid hash headers")
	}
	if _, err = os.Stat(filepath.Join(root+".states", "file-hashes.json")); err != nil {
		t.Fatal("manifest missing", err)
	}
}
func TestDownloadHashInvalidatesExternalReplacement(t *testing.T) {
	ts, root := newTestHTTPServer(t, "")
	target := filepath.Join(root, "file")
	os.WriteFile(target, []byte("old"), 0600)
	info, _ := os.Stat(target)
	first, err := ts.Client().Head(ts.URL + "/download/file")
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	os.WriteFile(target, []byte("new"), 0600)
	os.Chtimes(target, info.ModTime(), info.ModTime())
	second, err := ts.Client().Head(ts.URL + "/download/file")
	if err != nil {
		t.Fatal(err)
	}
	second.Body.Close()
	if second.Header.Get("X-File-SHA256") == first.Header.Get("X-File-SHA256") {
		t.Fatal("stale hash")
	}
}
