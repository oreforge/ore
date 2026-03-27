package build

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func CacheKey(softwareID, buildID, serverDir string) (string, error) {
	dirHash, err := hashDirectory(serverDir)
	if err != nil {
		return "", fmt.Errorf("hashing directory %s: %w", serverDir, err)
	}

	h := sha256.New()
	h.Write([]byte(softwareID))
	h.Write([]byte{0})
	h.Write([]byte(buildID))
	h.Write([]byte{0})
	h.Write([]byte(dirHash))

	return fmt.Sprintf("%x", h.Sum(nil))[:12], nil
}

func hashDirectory(dir string) (string, error) {
	var entries []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		entries = append(entries, rel)
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(entries)

	h := sha256.New()
	for _, entry := range entries {
		h.Write([]byte(entry))
		h.Write([]byte{0})
		if err := hashFile(h, filepath.Join(dir, entry)); err != nil {
			return "", err
		}
		h.Write([]byte{0})
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func hashFile(h hash.Hash, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(h, f)
	return err
}
