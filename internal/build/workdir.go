package build

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type WorkDir struct {
	root     string
	logger   *slog.Logger
	manifest *Manifest
}

func NewWorkDir(repoRoot string, logger *slog.Logger) (*WorkDir, error) {
	root := filepath.Join(repoRoot, ".ore")

	for _, dir := range []string{
		root,
		filepath.Join(root, "cache"),
		filepath.Join(root, "builds"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	return &WorkDir{
		root:     root,
		logger:   logger,
		manifest: LoadManifest(filepath.Join(root, "manifest.json")),
	}, nil
}

func (w *WorkDir) Root() string {
	return w.root
}

func (w *WorkDir) binaryPath(softwareID, filename string) string {
	dirName := strings.ReplaceAll(softwareID, ":", "-")
	return filepath.Join(w.root, "cache", dirName, filename)
}

func (w *WorkDir) HasBinary(sha256hex string) bool {
	if sha256hex == "" {
		return false
	}
	entry, ok := w.manifest.Binaries[sha256hex]
	if !ok {
		return false
	}
	_, err := os.Stat(w.binaryPath(entry.SoftwareID, entry.Filename))
	return err == nil
}

func (w *WorkDir) StoreBinary(sha256hex string, data []byte, softwareID, url string) error {
	filename := filenameFromURL(url)

	dir := filepath.Dir(w.binaryPath(softwareID, filename))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	target := w.binaryPath(softwareID, filename)
	tmp := target + ".tmp"

	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing cache temp file: %w", err)
	}

	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming to cache: %w", err)
	}

	w.manifest.Binaries[sha256hex] = BinaryEntry{
		SoftwareID: softwareID,
		Filename:   filename,
		SHA256:     sha256hex,
		URL:        url,
		Size:       int64(len(data)),
		CachedAt:   time.Now(),
	}

	shortHash := sha256hex
	if len(shortHash) > 12 {
		shortHash = shortHash[:12]
	}
	w.logger.Debug("cached binary", "sha256", shortHash, "software", softwareID, "file", filename, "size", len(data))
	return nil
}

func (w *WorkDir) ReadBinary(sha256hex string) ([]byte, error) {
	entry, ok := w.manifest.Binaries[sha256hex]
	if !ok {
		return nil, fmt.Errorf("binary %s not in manifest", sha256hex[:12])
	}
	return os.ReadFile(w.binaryPath(entry.SoftwareID, entry.Filename))
}

func filenameFromURL(url string) string {
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		if idx := strings.Index(name, "?"); idx >= 0 {
			name = name[:idx]
		}
		if name != "" {
			return name
		}
	}
	return "binary"
}

func (w *WorkDir) BuildDir(serverName, cacheKey string) (string, error) {
	dir := filepath.Join(w.root, "builds", serverName+"-"+cacheKey)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func (w *WorkDir) CleanOldBuilds(serverName, currentCacheKey string) {
	buildsDir := filepath.Join(w.root, "builds")
	entries, err := os.ReadDir(buildsDir)
	if err != nil {
		return
	}

	prefix := serverName + "-"
	currentDir := serverName + "-" + currentCacheKey

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) > len(prefix) && name[:len(prefix)] == prefix && name != currentDir {
			_ = os.RemoveAll(filepath.Join(buildsDir, name))
			delete(w.manifest.Builds, name)
		}
	}
}

func (w *WorkDir) WriteDockerfile(serverName, cacheKey, content string) error {
	dir, err := w.BuildDir(serverName, cacheKey)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(content), 0o644)
}

func (w *WorkDir) WriteDataDir(serverName, cacheKey, serverDir string) error {
	dir, err := w.BuildDir(serverName, cacheKey)
	if err != nil {
		return err
	}
	destDir := filepath.Join(dir, "data")
	_ = os.RemoveAll(destDir)
	return copyDir(serverDir, destDir)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func (w *WorkDir) WriteBinary(serverName, cacheKey, name string, data []byte, mode os.FileMode) error {
	dir, err := w.BuildDir(serverName, cacheKey)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), data, mode)
}

func (w *WorkDir) WriteEntrypoint(serverName, cacheKey string, data []byte) error {
	dir, err := w.BuildDir(serverName, cacheKey)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "entrypoint.sh"), data, 0o755)
}

func (w *WorkDir) CreateBuildLog(serverName, cacheKey string) (io.WriteCloser, error) {
	dir, err := w.BuildDir(serverName, cacheKey)
	if err != nil {
		return nil, err
	}
	return os.Create(filepath.Join(dir, "build.log"))
}

func (w *WorkDir) WriteMetadata(serverName, cacheKey string, meta Metadata) error {
	dir, err := w.BuildDir(serverName, cacheKey)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644); err != nil {
		return err
	}

	w.manifest.Builds[serverName+"-"+cacheKey] = Entry{
		ServerName: serverName,
		ImageTag:   meta.ImageTag,
		CacheKey:   cacheKey,
		SoftwareID: meta.SoftwareID,
		BuiltAt:    meta.StartedAt,
		DurationMs: meta.DurationMs,
	}

	return nil
}

func (w *WorkDir) SaveManifest() error {
	return w.manifest.Save(filepath.Join(w.root, "manifest.json"))
}

func (w *WorkDir) Manifest() *Manifest {
	return w.manifest
}

func (w *WorkDir) Clean() error {
	w.logger.Info("removing .ore directory", "path", w.root)
	return os.RemoveAll(w.root)
}

func (w *WorkDir) CleanCache() error {
	w.logger.Info("removing cached binaries")
	if err := os.RemoveAll(filepath.Join(w.root, "cache")); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(w.root, "cache"), 0o755); err != nil {
		return err
	}
	w.manifest.Binaries = make(map[string]BinaryEntry)
	return w.SaveManifest()
}

func (w *WorkDir) CleanBuilds() error {
	w.logger.Info("removing build artifacts")
	if err := os.RemoveAll(filepath.Join(w.root, "builds")); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(w.root, "builds"), 0o755); err != nil {
		return err
	}
	w.manifest.Builds = make(map[string]Entry)
	return w.SaveManifest()
}

func (w *WorkDir) Prune(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	w.logger.Info("pruning artifacts", "older_than", maxAge, "cutoff", cutoff)

	var buildKeys []string
	for key, entry := range w.manifest.Builds {
		if entry.BuiltAt.Before(cutoff) {
			dir := filepath.Join(w.root, "builds", entry.ServerName+"-"+entry.CacheKey)
			_ = os.RemoveAll(dir)
			buildKeys = append(buildKeys, key)
		}
	}
	for _, key := range buildKeys {
		delete(w.manifest.Builds, key)
	}

	var binaryKeys []string
	for hash, entry := range w.manifest.Binaries {
		if entry.CachedAt.Before(cutoff) {
			_ = os.Remove(w.binaryPath(entry.SoftwareID, entry.Filename))
			dirName := strings.ReplaceAll(entry.SoftwareID, ":", "-")
			_ = os.Remove(filepath.Join(w.root, "cache", dirName))
			binaryKeys = append(binaryKeys, hash)
		}
	}
	for _, key := range binaryKeys {
		delete(w.manifest.Binaries, key)
	}

	return w.SaveManifest()
}
