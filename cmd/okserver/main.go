package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/winezer0/okserver/cmd/client"
	"github.com/winezer0/okserver/internal/buildinfo"
	"github.com/winezer0/okserver/internal/server"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "-version" || os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(buildinfo.String())
		return
	}
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}

	if len(os.Args) >= 2 && os.Args[1] == "cli" {
		err = client.Run(exe, os.Args[2:])
	} else {
		err = server.Run(exe, os.Args[1:])
	}
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(1)
	}
}
