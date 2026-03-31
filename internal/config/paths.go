package config

import (
	"path/filepath"

	"github.com/adrg/xdg"
)

func OreConfigDir() string {
	return filepath.Join(xdg.ConfigHome, "ore")
}

func OredConfigDir() string {
	return filepath.Join(xdg.ConfigHome, "ored")
}

func OredDataDir() string {
	return filepath.Join(xdg.DataHome, "ored")
}

func OredProjectsDir() string {
	return filepath.Join(OredDataDir(), "projects")
}

func OreConfigFile() string {
	return filepath.Join(OreConfigDir(), "config.yaml")
}

func OredConfigFile() string {
	return filepath.Join(OredConfigDir(), "config.yaml")
}
