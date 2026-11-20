package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/kong"
	transport "github.com/winezer0/okserver/internal/client"
)

type cacheOptions struct {
	Directory string        `name:"dir" default:"." help:"download destination directory"`
	Prune     bool          `help:"remove inactive expired downloads"`
	Cancel    string        `help:"cancel one inactive download by ID"`
	Age       time.Duration `name:"older-than" default:"168h" help:"minimum inactive age for pruning"`
}

func (o *cacheOptions) Validate() error {
	if o.Age < 0 {
		return fmt.Errorf("older-than must be non-negative")
	}
	if o.Prune && o.Cancel != "" {
		return fmt.Errorf("--prune and --cancel cannot be used together")
	}
	return nil
}

func runDownloadCache(exe string, args []string) error {
	options := cacheOptions{}
	showHelp := false
	parser, err := kong.New(&options,
		kong.Name(exe+" cli cache"),
		kong.Description("Inspect and clean cached downloads."),
		kong.Exit(func(int) { showHelp = true }),
		kong.Writers(os.Stdout, os.Stderr),
	)
	if err != nil {
		return err
	}
	parsed, err := parser.Parse(args)
	if err != nil {
		if showHelp {
			return nil
		}
		printParseUsage(err, parsed)
		return err
	}
	if showHelp {
		return nil
	}
	cache := filepath.Join(options.Directory, ".okserver-downloads")
	if options.Cancel != "" {
		return transport.CancelCachedDownload(cache, options.Cancel)
	}
	if options.Prune {
		return transport.PruneDownloadCache(cache, options.Age)
	}
	tasks, err := transport.ListDownloadCache(cache)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(tasks)
}
