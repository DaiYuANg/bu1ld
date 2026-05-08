package pluginregistry

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEmbeddedRegistry(t *testing.T) {
	t.Parallel()

	index, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}

	goPlugin, ok := index.Find("org.bu1ld.go")
	if !ok {
		t.Fatalf("embedded registry missing org.bu1ld.go")
	}
	if got, want := goPlugin.Namespace, "go"; got != want {
		t.Fatalf("Namespace = %q, want %q", got, want)
	}
	latest, ok := goPlugin.LatestVersion()
	if !ok {
		t.Fatalf("org.bu1ld.go has no latest version")
	}
	if got, want := latest.Version, "0.1.3"; got != want {
		t.Fatalf("Version = %q, want %q", got, want)
	}
}

func TestLoadExternalRegistryResolvesRelativeAssets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "plugins.toml"), `
version = 1

[[plugins]]
id = "org.example.echo"
file = "plugins/org.example.echo.toml"
`)
	writeFile(t, filepath.Join(root, "plugins", "org.example.echo.toml"), `
id = "org.example.echo"
namespace = "echo"

[[versions]]
version = "0.1.0"

[[versions.assets]]
url = "../assets/echo"
format = "dir"
`)

	index, err := Load(context.Background(), LoadOptions{Source: root})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	plugin, ok := index.Find("org.example.echo")
	if !ok {
		t.Fatalf("registry missing org.example.echo")
	}
	version, ok := plugin.LatestVersion()
	if !ok {
		t.Fatalf("org.example.echo has no latest version")
	}
	asset, ok := version.Asset("", "")
	if !ok {
		t.Fatalf("org.example.echo has no generic asset")
	}
	want := filepath.Join(root, "assets", "echo")
	if got := filepath.Clean(asset.URL); got != want {
		t.Fatalf("Asset URL = %q, want %q", got, want)
	}
}

func TestLoadHTTPRegistryDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "registry", "plugins.toml"), `
version = 1

[[plugins]]
id = "org.example.echo"
file = "plugins/org.example.echo.toml"
`)
	writeFile(t, filepath.Join(root, "registry", "plugins", "org.example.echo.toml"), `
id = "org.example.echo"
namespace = "echo"

[[versions]]
version = "0.1.0"
`)
	server := httptest.NewServer(http.FileServer(http.Dir(root)))
	t.Cleanup(server.Close)

	index, err := Load(context.Background(), LoadOptions{Source: server.URL + "/registry"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, ok := index.Find("org.example.echo"); !ok {
		t.Fatalf("registry missing org.example.echo")
	}
}

func TestParseGitSource(t *testing.T) {
	t.Parallel()

	source, err := ParseSource("git+https://example.com/platform/bu1ld-plugins.git?ref=v1.2.3&path=registry")
	if err != nil {
		t.Fatalf("ParseSource() error = %v", err)
	}
	if got, want := source.Kind, SourceGit; got != want {
		t.Fatalf("Kind = %q, want %q", got, want)
	}
	if got, want := source.Location, "https://example.com/platform/bu1ld-plugins.git"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
	if got, want := source.Ref, "v1.2.3"; got != want {
		t.Fatalf("Ref = %q, want %q", got, want)
	}
	if got, want := filepath.ToSlash(source.Path), "registry"; got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestLoadGitRegistryKeepsMetadataSeparateFromAssets(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git executable is required for git registry test: %v", err)
	}

	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "registry", "plugins.toml"), `
version = 1

[[plugins]]
id = "org.example.echo"
file = "plugins/org.example.echo.toml"
`)
	writeFile(t, filepath.Join(repo, "registry", "plugins", "org.example.echo.toml"), `
id = "org.example.echo"
namespace = "echo"

[[versions]]
version = "0.1.0"

[[versions.assets]]
url = "https://downloads.example.com/bu1ld/echo/0.1.0/echo-linux-amd64.tar.gz"
format = "tar.gz"
os = "linux"
arch = "amd64"
`)
	runGitTest(t, repo, "init")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")
	runGitTest(t, repo, "add", "registry")
	runGitTest(t, repo, "commit", "-m", "registry")
	commit := runGitTest(t, repo, "rev-parse", "HEAD")

	index, err := Load(context.Background(), LoadOptions{
		Source:   "git+" + gitFileURL(repo) + "?ref=" + url.QueryEscape(commit) + "&path=registry",
		CacheDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	plugin, ok := index.Find("org.example.echo")
	if !ok {
		t.Fatalf("registry missing org.example.echo")
	}
	version, ok := plugin.LatestVersion()
	if !ok {
		t.Fatalf("org.example.echo has no latest version")
	}
	asset, ok := version.Asset("linux", "amd64")
	if !ok {
		t.Fatalf("org.example.echo has no linux/amd64 asset")
	}
	want := "https://downloads.example.com/bu1ld/echo/0.1.0/echo-linux-amd64.tar.gz"
	if got := asset.URL; got != want {
		t.Fatalf("Asset URL = %q, want %q", got, want)
	}
}

func TestSelectSkipsPendingRegistryVersions(t *testing.T) {
	t.Parallel()

	index, err := newIndex(1, nil, []Plugin{
		{
			ID:        "org.example.echo",
			Namespace: "echo",
			Versions: []PluginVersion{
				{Version: "0.2.0", Status: "pending"},
				{Version: "0.1.0", Status: "approved"},
			},
		},
	})
	if err != nil {
		t.Fatalf("newIndex() error = %v", err)
	}
	_, latest, err := Select(index, "org.example.echo")
	if err != nil {
		t.Fatalf("Select(latest) error = %v", err)
	}
	if got, want := latest.Version, "0.1.0"; got != want {
		t.Fatalf("latest Version = %q, want %q", got, want)
	}
	if _, _, err := Select(index, "org.example.echo@0.2.0"); err == nil {
		t.Fatalf("Select(pending) error = nil, want error")
	}
}

func TestValidateIndexReportsRejectedVersions(t *testing.T) {
	t.Parallel()

	index, err := newIndex(1, nil, []Plugin{
		{
			ID:        "org.example.echo",
			Namespace: "echo",
			Versions: []PluginVersion{
				{
					Version: "0.2.0",
					Status:  "approved",
					Assets: []PluginAsset{
						{OS: "linux", Arch: "amd64", URL: "echo.tar.gz", Format: "tar.gz"},
					},
				},
				{Version: "0.1.0", Status: "rejected"},
			},
		},
	})
	if err != nil {
		t.Fatalf("newIndex() error = %v", err)
	}
	report, err := ValidateIndex(index)
	if err != nil {
		t.Fatalf("ValidateIndex() error = %v", err)
	}
	if got, want := report.ApprovedVersions, 1; got != want {
		t.Fatalf("ApprovedVersions = %d, want %d", got, want)
	}
	if got, want := report.RejectedVersions, 1; got != want {
		t.Fatalf("RejectedVersions = %d, want %d", got, want)
	}
}

func TestValidateIndexRejectsApprovedVersionWithoutAssets(t *testing.T) {
	t.Parallel()

	index, err := newIndex(1, nil, []Plugin{
		{
			ID:        "org.example.echo",
			Namespace: "echo",
			Versions: []PluginVersion{
				{Version: "0.1.0", Status: "approved"},
			},
		},
	})
	if err != nil {
		t.Fatalf("newIndex() error = %v", err)
	}
	if _, err := ValidateIndex(index); err == nil {
		t.Fatalf("ValidateIndex() error = nil, want error")
	}
}

func TestVerifyAssetSignatureEd25519(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	payload := []byte("signed plugin asset")
	signature := ed25519.Sign(privateKey, payload)
	root := t.TempDir()
	assetPath := filepath.Join(root, "plugin.tar.gz")
	signaturePath := filepath.Join(root, "plugin.tar.gz.sig")
	writeFile(t, assetPath, string(payload))
	if err := os.WriteFile(signaturePath, signature, 0o600); err != nil {
		t.Fatalf("write signature: %v", err)
	}
	file, err := os.Open(assetPath)
	if err != nil {
		t.Fatalf("Open(asset) error = %v", err)
	}
	defer file.Close()

	err = verifyAssetSignatures(context.Background(), http.DefaultClient, PluginAsset{
		URL: assetPath,
		Signatures: []PluginSignature{
			{KeyID: "release", URL: signaturePath},
		},
	}, map[string]TrustedKey{
		"release": {
			ID:        "release",
			Algorithm: "ed25519",
			PublicKey: base64.StdEncoding.EncodeToString(publicKey),
		},
	}, file)
	if err != nil {
		t.Fatalf("verifyAssetSignatures() error = %v", err)
	}
}

func TestInstallDirAsset(t *testing.T) {
	t.Parallel()

	assetDir := t.TempDir()
	writeFile(t, filepath.Join(assetDir, "plugin.toml"), `
id = "org.example.echo"
namespace = "echo"
version = "0.1.0"
binary = "echo"
`)
	writeFile(t, filepath.Join(assetDir, "echo"), "#!/bin/sh\n")

	index, err := newIndex(1, nil, []Plugin{
		{
			ID:        "org.example.echo",
			Namespace: "echo",
			Versions: []PluginVersion{
				{
					Version: "0.1.0",
					Assets: []PluginAsset{
						{OS: "testos", Arch: "testarch", URL: assetDir, Format: "dir"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("newIndex() error = %v", err)
	}

	result, err := Install(context.Background(), index, InstallOptions{
		Ref:    "org.example.echo",
		Root:   t.TempDir(),
		GOOS:   "testos",
		GOARCH: "testarch",
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if got, want := result.ID, "org.example.echo"; got != want {
		t.Fatalf("ID = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "plugin.toml")); err != nil {
		t.Fatalf("installed manifest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "echo")); err != nil {
		t.Fatalf("installed binary missing: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}

func gitFileURL(path string) string {
	slash := filepath.ToSlash(path)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return (&url.URL{Scheme: "file", Path: slash}).String()
}
