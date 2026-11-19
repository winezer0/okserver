package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Protect the boundaries most likely to regress as new features are added.
func TestPackageDependencyBoundaries(t *testing.T) {
	root := filepath.Join("..", "..")
	const local = "github.com/winezer0/okserver/"
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(filepath.Join(root, "internal"), name)
		if err != nil {
			return err
		}
		owner := strings.Split(filepath.ToSlash(relative), "/")[0]
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, item := range file.Imports {
			dependency, err := strconv.Unquote(item.Path.Value)
			if err != nil {
				return err
			}
			forbidden := false
			switch owner {
			case "upload", "uploadstore", "storage", "digest", "diskbudget":
				forbidden = dependency == "net/http" || strings.HasPrefix(dependency, "github.com/gin-gonic/") || strings.HasPrefix(dependency, local+"cmd/") || dependency == local+"internal/server" || dependency == local+"internal/client" || dependency == local+"internal/protocol"
			case "client":
				forbidden = strings.HasPrefix(dependency, local+"cmd/") || dependency == local+"internal/server" || dependency == local+"internal/upload" || dependency == local+"internal/uploadstore"
			case "protocol":
				forbidden = strings.HasPrefix(dependency, local) || strings.HasPrefix(dependency, "github.com/gin-gonic/")
			}
			if forbidden {
				t.Errorf("%s imports forbidden dependency %s", relative, dependency)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
