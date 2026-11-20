package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/winezer0/okserver/cmd/client"
	"github.com/winezer0/okserver/internal/buildinfo"
	"github.com/winezer0/okserver/internal/server"
)

// version is set by release builds using -ldflags -X main.version=....
var version = "dev"

func main() {
	if run(os.Args[1:]) {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}

	args := os.Args[1:]
	if len(args) >= 1 && args[0] == "cli" {
		err = client.Run(exe, args[1:])
	} else {
		err = server.Run(exe, args)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(1)
	}
}

// run parses the root version flag before either command can load configuration.
func run(args []string) bool {
	if len(args) == 0 || (args[0] != "--version" && args[0] != "-v") {
		return false
	}
	var cli struct {
		Version kong.VersionFlag `short:"v" help:"Print version and exit."`
	}
	kong.Parse(&cli, kong.Name("okserver"), kong.Vars{"version": buildinfo.String(version)}, kong.Exit(func(int) {}), kong.Writers(os.Stdout, os.Stderr))
	return true
}
