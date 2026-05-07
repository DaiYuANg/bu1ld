package pluginregistry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

type SourceKind string

const (
	SourceEmbedded SourceKind = "embedded"
	SourceLocal    SourceKind = "local"
	SourceHTTP     SourceKind = "http"
	SourceGit      SourceKind = "git"
)

type Source struct {
	Kind     SourceKind
	Raw      string
	Location string
	Ref      string
	Path     string
}

func ParseSource(raw string) (Source, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "embedded" || raw == "embedded:" {
		return Source{Kind: SourceEmbedded, Raw: raw}, nil
	}
	if strings.HasPrefix(raw, "git+") {
		return parseGitSource(raw)
	}
	if isHTTPURL(raw) {
		return Source{Kind: SourceHTTP, Raw: raw, Location: raw}, nil
	}
	return Source{Kind: SourceLocal, Raw: raw, Location: raw}, nil
}

func parseGitSource(raw string) (Source, error) {
	value := strings.TrimPrefix(raw, "git+")
	location, query, _ := strings.Cut(value, "?")
	location = strings.TrimSpace(location)
	if location == "" {
		return Source{}, oops.In("bu1ld.plugin_registry").
			With("source", raw).
			New("git registry source location is required")
	}

	if parsed, err := url.Parse(location); err == nil && parsed.Fragment != "" {
		if query == "" {
			query = "ref=" + url.QueryEscape(parsed.Fragment)
		}
		parsed.Fragment = ""
		location = parsed.String()
	}

	values, err := url.ParseQuery(query)
	if err != nil {
		return Source{}, oops.In("bu1ld.plugin_registry").
			With("source", raw).
			Wrapf(err, "parse git registry source query")
	}
	path, err := cleanRegistrySubdir(values.Get("path"))
	if err != nil {
		return Source{}, err
	}
	return Source{
		Kind:     SourceGit,
		Raw:      raw,
		Location: location,
		Ref:      values.Get("ref"),
		Path:     path,
	}, nil
}

func materializeSource(ctx context.Context, source Source, options LoadOptions) (string, error) {
	switch source.Kind {
	case SourceLocal:
		if options.BaseDir != "" && !filepath.IsAbs(source.Location) {
			return filepath.Join(options.BaseDir, source.Location), nil
		}
		return source.Location, nil
	case SourceHTTP:
		return registryURL(source.Location)
	case SourceGit:
		return materializeGitSource(ctx, source, options.CacheDir)
	default:
		return "", oops.In("bu1ld.plugin_registry").
			With("source", source.Raw).
			With("kind", source.Kind).
			Errorf("unsupported plugin registry source kind %q", source.Kind)
	}
}

func materializeGitSource(ctx context.Context, source Source, cacheDir string) (string, error) {
	if cacheDir == "" {
		cacheDir = filepath.Join(".bu1ld", "registries")
	}
	repoDir := filepath.Join(cacheDir, gitSourceCacheKey(source.Location+"\x00"+strings.TrimSpace(source.Ref)))
	if _, err := exec.LookPath("git"); err != nil {
		return "", oops.In("bu1ld.plugin_registry").
			With("source", source.Location).
			Wrapf(err, "find git executable")
	}

	if isGitCheckout(repoDir) {
		if err := runGit(ctx, repoDir, "remote", "set-url", "origin", source.Location); err != nil {
			return "", err
		}
		if err := runGit(ctx, repoDir, "fetch", "--quiet", "--force", "--tags", "origin"); err != nil {
			return "", err
		}
		if strings.TrimSpace(source.Ref) != "" {
			if err := checkoutGitRef(ctx, repoDir, source.Ref); err != nil {
				return "", err
			}
		} else if err := runGit(ctx, repoDir, "pull", "--ff-only", "--quiet"); err != nil {
			return "", err
		}
	} else {
		if _, err := os.Stat(repoDir); err == nil {
			return "", oops.In("bu1ld.plugin_registry").
				With("path", repoDir).
				New("plugin registry cache path exists but is not a git checkout")
		} else if !os.IsNotExist(err) {
			return "", oops.In("bu1ld.plugin_registry").
				With("path", repoDir).
				Wrapf(err, "stat plugin registry cache path")
		}
		if err := os.MkdirAll(filepath.Dir(repoDir), 0o750); err != nil {
			return "", oops.In("bu1ld.plugin_registry").
				With("path", filepath.Dir(repoDir)).
				Wrapf(err, "create plugin registry cache dir")
		}
		if err := runGit(ctx, "", "clone", "--quiet", source.Location, repoDir); err != nil {
			return "", err
		}
		if strings.TrimSpace(source.Ref) != "" {
			if err := checkoutGitRef(ctx, repoDir, source.Ref); err != nil {
				return "", err
			}
		}
	}

	if source.Path == "" {
		return repoDir, nil
	}
	root, err := safeRegistryJoin(repoDir, source.Path)
	if err != nil {
		return "", err
	}
	return root, nil
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(output.String())
		if text == "" {
			return oops.In("bu1ld.plugin_registry").
				With("args", strings.Join(args, " ")).
				Wrapf(err, "run git")
		}
		return oops.In("bu1ld.plugin_registry").
			With("args", strings.Join(args, " ")).
			With("output", text).
			Wrapf(err, "run git")
	}
	return nil
}

func checkoutGitRef(ctx context.Context, repoDir, ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	remoteRef := "refs/remotes/origin/" + ref
	if gitRefExists(ctx, repoDir, remoteRef) {
		return runGit(ctx, repoDir, "checkout", "--quiet", "--detach", "origin/"+ref)
	}
	return runGit(ctx, repoDir, "checkout", "--quiet", "--detach", ref)
}

func gitRefExists(ctx context.Context, repoDir, ref string) bool {
	cmd := exec.CommandContext(ctx, "git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = repoDir
	return cmd.Run() == nil
}

func isGitCheckout(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func gitSourceCacheKey(location string) string {
	sum := sha256.Sum256([]byte(location))
	return hex.EncodeToString(sum[:])
}

func cleanRegistrySubdir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		return "", nil
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("registry source path %q escapes the registry root", path)
	}
	return clean, nil
}

func safeRegistryJoin(root, child string) (string, error) {
	base, err := filepath.Abs(root)
	if err != nil {
		return "", oops.In("bu1ld.plugin_registry").
			With("path", root).
			Wrapf(err, "resolve registry root")
	}
	path := filepath.Join(base, child)
	relative, err := filepath.Rel(base, path)
	if err != nil {
		return "", oops.In("bu1ld.plugin_registry").
			With("path", path).
			Wrapf(err, "validate registry source path")
	}
	if relative == "." || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return "", fmt.Errorf("registry source path %q escapes the registry root", child)
	}
	return path, nil
}
