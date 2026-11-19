package filebrowser

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestReadPageRejectsInvalidBounds(t *testing.T) {
	root := t.TempDir()
	for _, values := range []url.Values{
		{"page": {"0"}}, {"page": {"1001"}}, {"page": {"99999999999999999999999"}},
		{"limit": {"-1"}}, {"limit": {"501"}}, {"limit": {"nan"}}, {"sort": {"unknown"}},
	} {
		if _, err := ReadPage(root, "", values); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%v: expected invalid query, got %v", values, err)
		}
	}
	if _, err := ReadPage(filepath.Join(root, "missing"), "", nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory: %v", err)
	}
}

func TestReadPageFiltersPrivateEntriesAndKeepsTotals(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".okserver", ".okserver-downloads", "folder"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range map[string]string{"alpha.txt": "123", "Beta.txt": "12345"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	page, err := ReadPage(root, "nested", url.Values{"search": {"BETA"}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Files != 2 || page.Folders != 1 || page.Bytes != 8 || len(page.Entries) != 1 || page.Entries[0].Path != "nested/Beta.txt" {
		t.Fatalf("filtered page: %+v", page)
	}
	page, err = ReadPage(root, "nested", url.Values{"page": {"1000"}, "limit": {"500"}})
	if err != nil || page.Total != 3 || page.Entries == nil || len(page.Entries) != 0 {
		t.Fatalf("empty page: %+v, %v", page, err)
	}
	page, err = ReadPage(root, "", url.Values{"sort": {"size"}, "direction": {"desc"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 3 || page.Entries[0].Name != "folder" || page.Entries[1].Name != "Beta.txt" || page.Entries[2].Name != "alpha.txt" {
		t.Fatalf("sorted page: %+v", page)
	}
}
