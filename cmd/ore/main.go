package main

import (
	"os"

	"github.com/oreforge/ore/internal/cli"
)

var (
	version   = "dev"
	commit    = ""
	buildDate = ""
)

func main() {
	os.Exit(cli.Run(os.Args, cli.BuildInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}))
}
