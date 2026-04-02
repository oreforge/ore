package main

import (
	"os"

	"github.com/oreforge/ore/internal/server"
)

var (
	version   = "dev"
	commit    = ""
	buildDate = ""
)

func main() {
	os.Exit(server.Run(os.Args, server.BuildInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}))
}
