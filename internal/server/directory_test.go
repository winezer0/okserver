package server

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/winezer0/okserver/internal/filebrowser"
)

func TestDirectoryPaginationSearchSort(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 1200; index++ {
		os.WriteFile(filepath.Join(root, fmt.Sprintf("entry-%04d.txt", index)), []byte("test"), 0600)
	}
	os.Mkdir(filepath.Join(root, "folder"), 0700)
	os.Mkdir(filepath.Join(root, ".okserver"), 0700)
	page, err := filebrowser.ReadPage(root, "", url.Values{"page": {"2"}, "limit": {"100"}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1201 || page.Files != 1200 || page.Folders != 1 || len(page.Entries) != 100 || page.Entries[0].Name != "entry-0099.txt" {
		t.Fatalf("invalid page: total=%d first=%v", page.Total, page.Entries[0])
	}
	found, err := filebrowser.ReadPage(root, "", url.Values{"search": {"entry-11"}, "sort": {"name"}, "direction": {"desc"}})
	if err != nil || found.Total != 100 || found.Entries[0].Name != "entry-1199.txt" {
		t.Fatalf("invalid search: %+v %v", found, err)
	}
	if _, err = filebrowser.ReadPage(root, "", url.Values{"limit": {"999999"}}); err == nil {
		t.Fatal("unbounded page accepted")
	}
}
func TestPrivateDirectoryAliasIsRejected(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".okserver"), 0700)
	os.WriteFile(filepath.Join(root, ".okserver", "secret"), []byte("secret"), 0600)
	if err := os.Symlink(filepath.Join(root, ".okserver"), filepath.Join(root, "alias")); err != nil {
		t.Skip(err)
	}
	server := &fileServer{path: root}
	if _, _, err := server.resolvePath("alias/secret", false); err == nil {
		t.Fatal("private file exposed by alias")
	}
}
func TestOperationalEndpointsRequireAuthentication(t *testing.T) {
	server, _ := newTestHTTPServer(t, "user:password")
	for _, endpoint := range []string{"/healthz", "/readyz", "/metrics"} {
		response, err := server.Client().Get(server.URL + endpoint)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatalf("%s unprotected", endpoint)
		}
		request, _ := http.NewRequest("GET", server.URL+endpoint, nil)
		request.SetBasicAuth("user", "password")
		response, err = server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("%s failed: %d", endpoint, response.StatusCode)
		}
	}
}
