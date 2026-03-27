package main

import (
	"os"

	"github.com/oreforge/ore/internal/service"
)

var (
	version   = "dev"
	commit    = ""
	buildDate = ""
)

func main() {
	os.Exit(service.Run(os.Args, service.BuildInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}))
}
