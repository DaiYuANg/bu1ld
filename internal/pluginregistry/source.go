package pluginregistry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
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

	if isGitCheckout(repoDir) {
		repo, err := git.PlainOpen(repoDir)
		if err != nil {
			return "", oops.In("bu1ld.plugin_registry").
				With("path", repoDir).
				Wrapf(err, "open cached plugin registry git repository")
		}
		if err := setGitOrigin(repo, source.Location); err != nil {
			return "", err
		}
		if err := fetchGitSource(ctx, repo); err != nil {
			return "", err
		}
		if err := checkoutGitSource(ctx, repo, source.Ref); err != nil {
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
		repo, err := git.PlainCloneContext(ctx, repoDir, false, &git.CloneOptions{
			URL:  source.Location,
			Tags: git.AllTags,
		})
		if err != nil {
			return "", oops.In("bu1ld.plugin_registry").
				With("source", source.Location).
				With("path", repoDir).
				Wrapf(err, "clone plugin registry git repository")
		}
		if err := checkoutGitSource(ctx, repo, source.Ref); err != nil {
			return "", err
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

func setGitOrigin(repo *git.Repository, location string) error {
	cfg, err := repo.Config()
	if err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("source", location).
			Wrapf(err, "read plugin registry git config")
	}
	if cfg.Remotes == nil {
		cfg.Remotes = map[string]*config.RemoteConfig{}
	}
	cfg.Remotes["origin"] = &config.RemoteConfig{
		Name: "origin",
		URLs: []string{location},
	}
	if err := repo.SetConfig(cfg); err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("source", location).
			Wrapf(err, "write plugin registry git origin")
	}
	return nil
}

func fetchGitSource(ctx context.Context, repo *git.Repository) error {
	err := repo.FetchContext(ctx, &git.FetchOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			"+refs/heads/*:refs/remotes/origin/*",
			"+refs/tags/*:refs/tags/*",
		},
		Tags:  git.AllTags,
		Force: true,
	})
	if err == nil || errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return oops.In("bu1ld.plugin_registry").Wrapf(err, "fetch plugin registry git repository")
}

func checkoutGitSource(ctx context.Context, repo *git.Repository, ref string) error {
	ref = strings.TrimSpace(ref)
	worktree, err := repo.Worktree()
	if err != nil {
		return oops.In("bu1ld.plugin_registry").Wrapf(err, "open plugin registry git worktree")
	}
	if ref == "" {
		if err := worktree.PullContext(ctx, &git.PullOptions{RemoteName: "origin", Force: true}); err != nil &&
			!errors.Is(err, git.NoErrAlreadyUpToDate) {
			return oops.In("bu1ld.plugin_registry").Wrapf(err, "pull plugin registry git repository")
		}
		return nil
	}
	hash, err := resolveGitRef(repo, ref)
	if err != nil {
		return err
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: hash, Force: true}); err != nil {
		return oops.In("bu1ld.plugin_registry").
			With("ref", ref).
			Wrapf(err, "checkout plugin registry git ref")
	}
	return nil
}

func resolveGitRef(repo *git.Repository, ref string) (plumbing.Hash, error) {
	if hash := plumbing.NewHash(ref); !hash.IsZero() {
		if _, err := repo.CommitObject(hash); err == nil {
			return hash, nil
		}
	}
	revisions := []plumbing.Revision{
		plumbing.Revision(ref),
		plumbing.Revision("refs/heads/" + ref),
		plumbing.Revision("refs/remotes/origin/" + ref),
		plumbing.Revision("refs/tags/" + ref),
	}
	var lastErr error
	for _, revision := range revisions {
		hash, err := repo.ResolveRevision(revision)
		if err == nil {
			return *hash, nil
		}
		lastErr = err
	}
	return plumbing.ZeroHash, oops.In("bu1ld.plugin_registry").
		With("ref", ref).
		Wrapf(lastErr, "resolve plugin registry git ref")
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
	path, err := securejoin.SecureJoin(base, child)
	if err != nil {
		return "", oops.In("bu1ld.plugin_registry").
			With("path", child).
			Wrapf(err, "resolve registry source path")
	}
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
