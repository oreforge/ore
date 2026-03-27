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
