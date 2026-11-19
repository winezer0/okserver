package client

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	transport "github.com/winezer0/okserver/internal/client"
)

func Run(exe string, args []string) error {
	if len(args) > 0 && args[0] == "cache" {
		return runDownloadCache(args[1:])
	}
	options := transport.Options{}
	flags := flag.NewFlagSet(exe+" cli", flag.ContinueOnError)
	flags.StringVar(&options.Data, "d", "", "@file to upload with automatic resume")
	flags.StringVar(&options.Output, "o", "", "output")
	flags.IntVar(&options.DownloadConcurrency, "parallel", 4, "download concurrency (1..16)")
	flags.BoolVar(&options.Resume, "c", false, "resume downloads (uploads resume automatically)")
	flags.StringVar(&options.CACert, "ca", "", "ca.crt to verify peer against")
	flags.BoolVar(&options.Insecure, "k", false, "allow insecure server connections")
	flags.DurationVar(&options.Timeout, "t", 5*time.Minute, "client timeout")
	flags.StringVar(&options.Auth, "auth", "", "username:password")
	if err := flags.Parse(args); err != nil {
		return err
	}
	options.URL = flags.Arg(0)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return transport.ExecuteContext(ctx, options)
}
