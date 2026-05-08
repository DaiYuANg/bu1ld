package pluginregistry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	buildplugin "bu1ld/internal/plugin"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/mholt/archives"
	"github.com/samber/oops"
)

type InstallOptions struct {
	Ref    string
	Root   string
	Force  bool
	GOOS   string
	GOARCH string
	Client *http.Client
}

type InstallResult struct {
	ID       string
	Version  string
	Path     string
	Manifest buildplugin.Manifest
}

func Install(ctx context.Context, index *Index, options InstallOptions) (InstallResult, error) {
	if index == nil {
		return InstallResult{}, fmt.Errorf("plugin registry is required")
	}
	if strings.TrimSpace(options.Root) == "" {
		return InstallResult{}, fmt.Errorf("plugin install root is required")
	}
	if options.Client == nil {
		options.Client = http.DefaultClient
	}

	plugin, version, err := Select(index, options.Ref)
	if err != nil {
		return InstallResult{}, err
	}
	asset, ok := version.AssetOption(options.GOOS, options.GOARCH).Get()
	if !ok {
		return InstallResult{}, fmt.Errorf("plugin %s@%s has no asset for %s/%s", plugin.ID, version.Version, options.GOOS, options.GOARCH)
	}

	tempDir, err := os.MkdirTemp("", "bu1ld-plugin-*")
	if err != nil {
		return InstallResult{}, oops.In("bu1ld.plugin_registry").Wrapf(err, "create plugin install temp dir")
	}
	defer os.RemoveAll(tempDir)

	extractedDir := filepath.Join(tempDir, "extract")
	if err := os.MkdirAll(extractedDir, 0o750); err != nil {
		return InstallResult{}, oops.In("bu1ld.plugin_registry").
			With("path", extractedDir).
			Wrapf(err, "create plugin extract dir")
	}
	if err := extractAsset(ctx, options.Client, asset, extractedDir); err != nil {
		return InstallResult{}, err
	}

	manifestPath, err := findManifestPath(extractedDir)
	if err != nil {
		return InstallResult{}, err
	}
	manifest, err := buildplugin.ReadManifest(manifestPath)
	if err != nil {
		return InstallResult{}, err
	}
	if manifest.ID != plugin.ID {
		return InstallResult{}, oops.In("bu1ld.plugin_registry").
			With("plugin", plugin.ID).
			With("manifest_id", manifest.ID).
			Errorf("registry plugin id %q does not match manifest id %q", plugin.ID, manifest.ID)
	}
	if manifest.Version != version.Version {
		return InstallResult{}, oops.In("bu1ld.plugin_registry").
			With("plugin", plugin.ID).
			With("version", version.Version).
			With("manifest_version", manifest.Version).
			Errorf("registry plugin version %q does not match manifest version %q", version.Version, manifest.Version)
	}

	targetDir, err := installTarget(options.Root, plugin.ID, version.Version)
	if err != nil {
		return InstallResult{}, err
	}
	if err := replaceInstallDir(filepath.Dir(manifestPath), targetDir, options.Force); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{
		ID:       plugin.ID,
		Version:  version.Version,
		Path:     targetDir,
		Manifest: manifest,
	}, nil
}

func extractAsset(ctx context.Context, client *http.Client, asset PluginAsset, target string) error {
	format := assetFormat(asset)
	if format == "dir" {
		source, err := localAssetPath(asset.URL)
		if err != nil {
			return err
		}
		return copyDir(source, target)
	}

	file, err := os.CreateTemp("", "bu1ld-plugin-asset-*")
	if err != nil {
		return oops.In("bu1ld.plugin_registry").Wrapf(err, "create plugin asset temp file")
	}
	path := file.Name()
	defer os.Remove(path)
	defer file.Close()

	if err := downloadAsset(ctx, client, asset.URL, file); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("file", path).
			Wrapf(err, "seek plugin asset")
	}
	if err := verifySHA256(file, asset.SHA256); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("file", path).
			Wrapf(err, "seek plugin asset")
	}

	switch format {
	case "zip":
		return extractArchive(ctx, path, file, target, archives.Zip{})
	case "tar":
		return extractArchive(ctx, path, file, target, archives.Tar{})
	case "tar.gz", "tgz":
		return extractArchive(ctx, path, file, target, archives.CompressedArchive{
			Compression: archives.Gz{},
			Extraction:  archives.Tar{},
		})
	default:
		return fmt.Errorf("unsupported plugin asset format %q", format)
	}
}

func downloadAsset(ctx context.Context, client *http.Client, source string, target io.Writer) error {
	if isHTTPURL(source) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return oops.In("bu1ld.plugin_registry").
				With("url", source).
				Wrapf(err, "create plugin asset request")
		}
		response, err := client.Do(request)
		if err != nil {
			return oops.In("bu1ld.plugin_registry").
				With("url", source).
				Wrapf(err, "download plugin asset")
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return oops.In("bu1ld.plugin_registry").
				With("url", source).
				With("status", response.Status).
				Errorf("download plugin asset returned %s", response.Status)
		}
		if _, err := io.Copy(target, response.Body); err != nil {
			return oops.In("bu1ld.plugin_registry").
				With("url", source).
				Wrapf(err, "write plugin asset")
		}
		return nil
	}

	path, err := localAssetPath(source)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("file", path).
			Wrapf(err, "open plugin asset")
	}
	defer file.Close()
	if _, err := io.Copy(target, file); err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("file", path).
			Wrapf(err, "copy plugin asset")
	}
	return nil
}

func verifySHA256(reader io.Reader, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	expected = strings.TrimPrefix(expected, "sha256:")
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return oops.In("bu1ld.plugin_registry").Wrapf(err, "hash plugin asset")
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("plugin asset checksum mismatch: got sha256:%s, want sha256:%s", actual, expected)
	}
	return nil
}

func extractArchive(ctx context.Context, assetPath string, reader io.Reader, target string, extractor archives.Extractor) error {
	return extractor.Extract(ctx, reader, func(_ context.Context, file archives.FileInfo) error {
		name := path.Clean(file.NameInArchive)
		if name == "." || name == "/" {
			return nil
		}
		destination, err := safeJoin(target, name)
		if err != nil {
			return err
		}
		mode := file.Mode()
		if file.IsDir() {
			if err := os.MkdirAll(destination, dirMode(mode)); err != nil {
				return oops.In("bu1ld.plugin_registry").
					With("path", destination).
					Wrapf(err, "create plugin asset directory")
			}
			return nil
		}
		if !mode.IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
			return oops.In("bu1ld.plugin_registry").
				With("path", filepath.Dir(destination)).
				Wrapf(err, "create plugin asset parent directory")
		}
		source, err := file.Open()
		if err != nil {
			return oops.In("bu1ld.plugin_registry").
				With("file", file.NameInArchive).
				With("archive", assetPath).
				Wrapf(err, "open plugin asset file")
		}
		err = writeFileFromFS(destination, source, mode)
		closeErr := source.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return oops.In("bu1ld.plugin_registry").
				With("file", file.NameInArchive).
				Wrapf(closeErr, "close plugin asset file")
		}
		return nil
	})
}

func findManifestPath(root string) (string, error) {
	rootManifest := filepath.Join(root, buildplugin.ManifestFileName)
	if info, err := os.Stat(rootManifest); err == nil && !info.IsDir() {
		return rootManifest, nil
	}

	var found string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != buildplugin.ManifestFileName {
			return nil
		}
		if found == "" || path < found {
			found = path
		}
		return nil
	}); err != nil {
		return "", oops.In("bu1ld.plugin_registry").
			With("path", root).
			Wrapf(err, "find plugin manifest")
	}
	if found == "" {
		return "", fmt.Errorf("plugin asset does not contain %s", buildplugin.ManifestFileName)
	}
	return found, nil
}

func installTarget(root, id, version string) (string, error) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", oops.In("bu1ld.plugin_registry").
			With("path", root).
			Wrapf(err, "resolve plugin install root")
	}
	target, err := securejoin.SecureJoin(cleanRoot, filepath.Join(id, version))
	if err != nil {
		return "", oops.In("bu1ld.plugin_registry").
			With("path", filepath.Join(id, version)).
			Wrapf(err, "resolve plugin install target")
	}
	relative, err := filepath.Rel(cleanRoot, target)
	if err != nil {
		return "", oops.In("bu1ld.plugin_registry").
			With("path", target).
			Wrapf(err, "validate plugin install target")
	}
	if relative == "." || strings.HasPrefix(relative, "..") {
		return "", fmt.Errorf("plugin install target %s is outside root %s", target, cleanRoot)
	}
	return target, nil
}

func replaceInstallDir(source, target string, force bool) error {
	if _, err := os.Stat(target); err == nil {
		if !force {
			return fmt.Errorf("plugin is already installed at %s", target)
		}
		if err := os.RemoveAll(target); err != nil {
			return oops.In("bu1ld.plugin_registry").
				With("path", target).
				Wrapf(err, "remove existing plugin install")
		}
	} else if !os.IsNotExist(err) {
		return oops.In("bu1ld.plugin_registry").
			With("path", target).
			Wrapf(err, "stat plugin install target")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("path", filepath.Dir(target)).
			Wrapf(err, "create plugin install parent")
	}
	return copyDir(source, target)
}

func copyDir(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return oops.In("bu1ld.plugin_registry").
				With("path", path).
				Wrapf(err, "copy plugin file")
		}
		destination := filepath.Join(target, relative)
		info, err := entry.Info()
		if err != nil {
			return oops.In("bu1ld.plugin_registry").
				With("path", path).
				Wrapf(err, "stat plugin file")
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, dirMode(info.Mode()))
		}
		return copyFile(path, destination, info.Mode())
	})
}

func copyFile(source, target string, mode os.FileMode) error {
	file, err := os.Open(source)
	if err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("file", source).
			Wrapf(err, "open plugin file")
	}
	defer file.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("path", filepath.Dir(target)).
			Wrapf(err, "create plugin file parent")
	}
	return writeFileFromReader(target, file, mode)
}

func writeFileFromReader(path string, reader io.Reader, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode(mode))
	if err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("file", path).
			Wrapf(err, "create plugin file")
	}
	if _, err := io.Copy(file, reader); err != nil {
		_ = file.Close()
		return oops.In("bu1ld.plugin_registry").
			With("file", path).
			Wrapf(err, "write plugin file")
	}
	if err := file.Close(); err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("file", path).
			Wrapf(err, "close plugin file")
	}
	return nil
}

func writeFileFromFS(path string, reader fs.File, mode os.FileMode) error {
	return writeFileFromReader(path, reader, mode)
}

func dirMode(mode os.FileMode) os.FileMode {
	if mode.Perm() == 0 {
		return 0o750
	}
	return mode
}

func fileMode(mode os.FileMode) os.FileMode {
	if mode.Perm() == 0 {
		return 0o600
	}
	return mode
}

func safeJoin(root, name string) (string, error) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path, err := securejoin.SecureJoin(cleanRoot, name)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(cleanRoot, path)
	if err != nil {
		return "", err
	}
	if relative == "." || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return "", fmt.Errorf("plugin asset path %q escapes extraction directory", name)
	}
	return path, nil
}

func assetFormat(asset PluginAsset) string {
	format := strings.ToLower(strings.TrimSpace(asset.Format))
	if format != "" {
		return format
	}
	url := strings.ToLower(asset.URL)
	switch {
	case strings.HasSuffix(url, ".zip"):
		return "zip"
	case strings.HasSuffix(url, ".tar.gz"):
		return "tar.gz"
	case strings.HasSuffix(url, ".tgz"):
		return "tgz"
	case strings.HasSuffix(url, ".tar"):
		return "tar"
	default:
		return ""
	}
}

func localAssetPath(source string) (string, error) {
	if strings.HasPrefix(source, "file://") {
		return fileURLPath(source)
	}
	if isHTTPURL(source) {
		return "", fmt.Errorf("asset %s is not a local path", source)
	}
	return filepath.FromSlash(source), nil
}
