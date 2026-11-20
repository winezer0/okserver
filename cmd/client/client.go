package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	transport "github.com/winezer0/okserver/internal/client"
)

func printParseUsage(err error, parsed *kong.Context) {
	if parsed == nil {
		var parseErr *kong.ParseError
		if errors.As(err, &parseErr) {
			parsed = parseErr.Context
		}
	}
	if parsed != nil {
		_ = parsed.PrintUsage(false)
	}
}

type TransferOptions struct {
	Data                string        `short:"d" help:"@file to upload with automatic resume"`
	Output              string        `short:"o" help:"output"`
	DownloadConcurrency int           `name:"parallel" default:"4" help:"download concurrency (1..16)"`
	Resume              bool          `short:"c" help:"resume downloads (uploads resume automatically)"`
	CACert              string        `name:"ca" help:"ca.crt to verify peer against"`
	Insecure            bool          `short:"k" help:"allow insecure server connections"`
	Timeout             time.Duration `short:"t" default:"5m" help:"client timeout"`
	Auth                string        `help:"username:password"`
	URL                 string        `arg:"" required:"" help:"server URL"`
}

func (o *TransferOptions) Validate() error {
	if o.DownloadConcurrency < 1 || o.DownloadConcurrency > 16 {
		return fmt.Errorf("parallel must be between 1 and 16")
	}
	return nil
}

func Run(exe string, args []string) error {
	if len(args) > 0 && args[0] == "cache" {
		return runDownloadCache(exe, args[1:])
	}
	options := TransferOptions{}
	showHelp := false
	parser, err := kong.New(&options,
		kong.Name(exe+" cli"),
		kong.Description("Upload and download files."),
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
	clientOptions := transport.Options{
		Data: options.Data, Output: options.Output,
		DownloadConcurrency: options.DownloadConcurrency, Resume: options.Resume,
		CACert: options.CACert, Insecure: options.Insecure, Timeout: options.Timeout,
		Auth: options.Auth, URL: options.URL,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return transport.ExecuteContext(ctx, clientOptions)
}
