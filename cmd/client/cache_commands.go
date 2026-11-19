package client

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	transport "github.com/winezer0/okserver/internal/client"
)

func runDownloadCache(args []string) error {
	flags := flag.NewFlagSet("cli cache", flag.ContinueOnError)
	directory := flags.String("dir", ".", "download destination directory")
	prune := flags.Bool("prune", false, "remove inactive expired downloads")
	cancel := flags.String("cancel", "", "cancel one inactive download by ID")
	age := flags.Duration("older-than", 7*24*time.Hour, "minimum inactive age for pruning")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cache := filepath.Join(*directory, ".okserver-downloads")
	if *age < 0 {
		return fmt.Errorf("older-than must be non-negative")
	}
	if *cancel != "" {
		return transport.CancelCachedDownload(cache, *cancel)
	}
	if *prune {
		return transport.PruneDownloadCache(cache, *age)
	}
	tasks, err := transport.ListDownloadCache(cache)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(tasks)
}
