package pluginregistry

import (
	"context"
	"embed"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/pelletier/go-toml/v2"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

const (
	defaultIndexFile = "plugins.toml"
	embeddedRoot     = "catalog"
)

//go:embed catalog/plugins.toml catalog/plugins/*.toml
var embeddedCatalog embed.FS

type LoadOptions struct {
	Source   string
	BaseDir  string
	CacheDir string
	Client   *http.Client
}

type Index struct {
	Version     int
	TrustedKeys []TrustedKey
	Items       []Plugin
}

type IndexFile struct {
	Version     int          `toml:"version"`
	TrustedKeys []TrustedKey `toml:"trusted_keys,omitempty"`
	Plugins     []PluginRef  `toml:"plugins"`
}

type PluginRef struct {
	ID   string `toml:"id"`
	File string `toml:"file"`
}

type Plugin struct {
	ID          string          `toml:"id"`
	Namespace   string          `toml:"namespace"`
	Owner       string          `toml:"owner,omitempty"`
	Description string          `toml:"description,omitempty"`
	Homepage    string          `toml:"homepage,omitempty"`
	Tags        []string        `toml:"tags,omitempty"`
	Versions    []PluginVersion `toml:"versions,omitempty"`
}

type PluginVersion struct {
	Version    string        `toml:"version"`
	Bu1ld      string        `toml:"bu1ld,omitempty"`
	Status     string        `toml:"status,omitempty"`
	ReviewedBy string        `toml:"reviewed_by,omitempty"`
	ReviewedAt string        `toml:"reviewed_at,omitempty"`
	Manifest   string        `toml:"manifest,omitempty"`
	Assets     []PluginAsset `toml:"assets,omitempty"`
}

type PluginAsset struct {
	OS         string            `toml:"os,omitempty"`
	Arch       string            `toml:"arch,omitempty"`
	URL        string            `toml:"url"`
	SHA256     string            `toml:"sha256,omitempty"`
	Format     string            `toml:"format,omitempty"`
	Signatures []PluginSignature `toml:"signatures,omitempty"`
}

type PluginSignature struct {
	KeyID     string `toml:"key_id"`
	Algorithm string `toml:"algorithm,omitempty"`
	URL       string `toml:"url"`
	SHA256    string `toml:"sha256,omitempty"`
}

type TrustedKey struct {
	ID        string `toml:"id"`
	Algorithm string `toml:"algorithm,omitempty"`
	PublicKey string `toml:"public_key"`
}

func Load(ctx context.Context, options LoadOptions) (*Index, error) {
	source, err := ParseSource(options.Source)
	if err != nil {
		return nil, err
	}
	if source.Kind == SourceEmbedded {
		return LoadEmbedded()
	}
	if options.Client == nil {
		options.Client = http.DefaultClient
	}
	root, err := materializeSource(ctx, source, options)
	if err != nil {
		return nil, err
	}
	return loadExternal(ctx, options, root)
}

func LoadEmbedded() (*Index, error) {
	indexPath := filepath.ToSlash(filepath.Join(embeddedRoot, defaultIndexFile))
	data, err := embeddedCatalog.ReadFile(indexPath)
	if err != nil {
		return nil, oops.In("bu1ld.plugin_registry").
			With("file", indexPath).
			Wrapf(err, "read embedded plugin registry")
	}
	indexFile, err := parseIndexFile(indexPath, data)
	if err != nil {
		return nil, err
	}

	plugins := list.NewListWithCapacity[Plugin](len(indexFile.Plugins))
	for _, ref := range indexFile.Plugins {
		path := filepath.ToSlash(filepath.Join(embeddedRoot, ref.File))
		data, err := embeddedCatalog.ReadFile(path)
		if err != nil {
			return nil, oops.In("bu1ld.plugin_registry").
				With("file", path).
				Wrapf(err, "read embedded plugin registry entry")
		}
		plugin, err := parsePluginFile(path, data)
		if err != nil {
			return nil, err
		}
		if plugin.ID != ref.ID {
			return nil, oops.In("bu1ld.plugin_registry").
				With("file", path).
				With("plugin", plugin.ID).
				With("index_plugin", ref.ID).
				Errorf("registry plugin id %q does not match index id %q", plugin.ID, ref.ID)
		}
		plugins.Add(plugin)
	}
	return newIndex(indexFile.Version, indexFile.TrustedKeys, plugins.Values())
}

func (i *Index) Search(query string) []Plugin {
	query = strings.ToLower(strings.TrimSpace(query))
	items := list.NewList(i.Items...)
	if query == "" {
		return items.Values()
	}
	return items.Where(func(_ int, item Plugin) bool {
		return item.matches(query)
	}).Values()
}

func (i *Index) Find(id string) (Plugin, bool) {
	return i.FindOption(id).Get()
}

func (i *Index) trustedKeyMap() map[string]TrustedKey {
	keys := make(map[string]TrustedKey, len(i.TrustedKeys))
	for _, key := range i.TrustedKeys {
		keys[strings.TrimSpace(key.ID)] = key
	}
	return keys
}

func (i *Index) FindOption(id string) mo.Option[Plugin] {
	return list.NewList(i.Items...).FirstWhere(func(_ int, item Plugin) bool {
		if item.ID == id {
			return true
		}
		return false
	})
}

func (p Plugin) LatestVersion() (PluginVersion, bool) {
	return p.LatestVersionOption().Get()
}

func (p Plugin) LatestVersionOption() mo.Option[PluginVersion] {
	return list.NewList(p.Versions...).FirstWhere(func(_ int, item PluginVersion) bool {
		return item.Approved()
	})
}

func (p Plugin) Version(version string) (PluginVersion, bool) {
	return p.VersionOption(version).Get()
}

func (p Plugin) VersionOption(version string) mo.Option[PluginVersion] {
	return list.NewList(p.Versions...).FirstWhere(func(_ int, item PluginVersion) bool {
		if item.Version == version && item.Approved() {
			return true
		}
		return false
	})
}

func (v PluginVersion) Asset(goos, goarch string) (PluginAsset, bool) {
	return v.AssetOption(goos, goarch).Get()
}

func (v PluginVersion) AssetOption(goos, goarch string) mo.Option[PluginAsset] {
	assets := list.NewList(v.Assets...)
	exact := assets.FirstWhere(func(_ int, asset PluginAsset) bool {
		if asset.OS == goos && asset.Arch == goarch {
			return true
		}
		return false
	})
	if exact.IsPresent() {
		return exact
	}
	return assets.FirstWhere(func(_ int, asset PluginAsset) bool {
		if asset.OS == "" && asset.Arch == "" {
			return true
		}
		return false
	})
}

func (v PluginVersion) Approved() bool {
	status := strings.TrimSpace(strings.ToLower(v.Status))
	return status == "" || status == "approved"
}

func ParseRef(ref string) (id string, version string, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", fmt.Errorf("plugin reference is required")
	}
	id, version, _ = strings.Cut(ref, "@")
	id = strings.TrimSpace(id)
	version = strings.TrimSpace(version)
	if id == "" {
		return "", "", fmt.Errorf("plugin id is required")
	}
	return id, version, nil
}

func Select(index *Index, ref string) (Plugin, PluginVersion, error) {
	id, version, err := ParseRef(ref)
	if err != nil {
		return Plugin{}, PluginVersion{}, err
	}
	plugin, ok := index.FindOption(id).Get()
	if !ok {
		return Plugin{}, PluginVersion{}, fmt.Errorf("plugin %q was not found in the registry", id)
	}
	if version != "" {
		item, ok := plugin.VersionOption(version).Get()
		if !ok {
			return Plugin{}, PluginVersion{}, fmt.Errorf("plugin %q version %q was not found in the registry", id, version)
		}
		return plugin, item, nil
	}
	item, ok := plugin.LatestVersionOption().Get()
	if !ok {
		return Plugin{}, PluginVersion{}, fmt.Errorf("plugin %q has no versions in the registry", id)
	}
	return plugin, item, nil
}

func loadExternal(ctx context.Context, options LoadOptions, source string) (*Index, error) {
	data, indexBase, err := readRegistryFile(ctx, options.Client, source)
	if err != nil {
		return nil, err
	}
	indexFile, err := parseIndexFile(source, data)
	if err != nil {
		return nil, err
	}

	plugins := list.NewListWithCapacity[Plugin](len(indexFile.Plugins))
	for _, ref := range indexFile.Plugins {
		entrySource, err := resolveRegistryRef(indexBase, ref.File)
		if err != nil {
			return nil, err
		}
		data, entryBase, err := readRegistryFile(ctx, options.Client, entrySource)
		if err != nil {
			return nil, err
		}
		plugin, err := parsePluginFile(entrySource, data)
		if err != nil {
			return nil, err
		}
		if plugin.ID != ref.ID {
			return nil, oops.In("bu1ld.plugin_registry").
				With("file", entrySource).
				With("plugin", plugin.ID).
				With("index_plugin", ref.ID).
				Errorf("registry plugin id %q does not match index id %q", plugin.ID, ref.ID)
		}
		resolvePluginAssets(&plugin, entryBase)
		plugins.Add(plugin)
	}
	return newIndex(indexFile.Version, indexFile.TrustedKeys, plugins.Values())
}

func readRegistryFile(ctx context.Context, client *http.Client, source string) ([]byte, string, error) {
	if isHTTPURL(source) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, "", oops.In("bu1ld.plugin_registry").
				With("url", source).
				Wrapf(err, "create plugin registry request")
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, "", oops.In("bu1ld.plugin_registry").
				With("url", source).
				Wrapf(err, "fetch plugin registry")
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, "", oops.In("bu1ld.plugin_registry").
				With("url", source).
				With("status", response.Status).
				Errorf("fetch plugin registry returned %s", response.Status)
		}
		data, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, "", oops.In("bu1ld.plugin_registry").
				With("url", source).
				Wrapf(err, "read plugin registry response")
		}
		return data, source, nil
	}

	path, err := registryPath(source)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", oops.In("bu1ld.plugin_registry").
			With("file", path).
			Wrapf(err, "read plugin registry")
	}
	return data, path, nil
}

func parseIndexFile(path string, data []byte) (IndexFile, error) {
	var index IndexFile
	if err := toml.Unmarshal(data, &index); err != nil {
		return IndexFile{}, oops.In("bu1ld.plugin_registry").
			With("file", path).
			Wrapf(err, "parse plugin registry")
	}
	if index.Version == 0 {
		return IndexFile{}, oops.In("bu1ld.plugin_registry").
			With("file", path).
			New("plugin registry version is required")
	}
	for _, plugin := range index.Plugins {
		if strings.TrimSpace(plugin.ID) == "" {
			return IndexFile{}, oops.In("bu1ld.plugin_registry").
				With("file", path).
				New("registry plugin id is required")
		}
		if strings.TrimSpace(plugin.File) == "" {
			return IndexFile{}, oops.In("bu1ld.plugin_registry").
				With("file", path).
				With("plugin", plugin.ID).
				New("registry plugin file is required")
		}
	}
	if err := validateTrustedKeys(path, index.TrustedKeys); err != nil {
		return IndexFile{}, err
	}
	return index, nil
}

func parsePluginFile(path string, data []byte) (Plugin, error) {
	var plugin Plugin
	if err := toml.Unmarshal(data, &plugin); err != nil {
		return Plugin{}, oops.In("bu1ld.plugin_registry").
			With("file", path).
			Wrapf(err, "parse plugin registry entry")
	}
	if strings.TrimSpace(plugin.ID) == "" {
		return Plugin{}, oops.In("bu1ld.plugin_registry").
			With("file", path).
			New("registry plugin id is required")
	}
	if strings.TrimSpace(plugin.Namespace) == "" {
		return Plugin{}, oops.In("bu1ld.plugin_registry").
			With("file", path).
			With("plugin", plugin.ID).
			New("registry plugin namespace is required")
	}
	for _, version := range plugin.Versions {
		if strings.TrimSpace(version.Version) == "" {
			return Plugin{}, oops.In("bu1ld.plugin_registry").
				With("file", path).
				With("plugin", plugin.ID).
				New("registry plugin version is required")
		}
		switch strings.TrimSpace(strings.ToLower(version.Status)) {
		case "", "approved", "pending", "rejected":
		default:
			return Plugin{}, oops.In("bu1ld.plugin_registry").
				With("file", path).
				With("plugin", plugin.ID).
				With("version", version.Version).
				Errorf("registry plugin version status %q is invalid", version.Status)
		}
		for _, asset := range version.Assets {
			for _, signature := range asset.Signatures {
				if strings.TrimSpace(signature.KeyID) == "" {
					return Plugin{}, oops.In("bu1ld.plugin_registry").
						With("file", path).
						With("plugin", plugin.ID).
						With("version", version.Version).
						New("registry plugin asset signature key_id is required")
				}
				if strings.TrimSpace(signature.URL) == "" {
					return Plugin{}, oops.In("bu1ld.plugin_registry").
						With("file", path).
						With("plugin", plugin.ID).
						With("version", version.Version).
						New("registry plugin asset signature url is required")
				}
				if !registrySignatureAlgorithmSupported(signature.Algorithm) {
					return Plugin{}, oops.In("bu1ld.plugin_registry").
						With("file", path).
						With("plugin", plugin.ID).
						With("version", version.Version).
						Errorf("registry plugin asset signature algorithm %q is unsupported", signature.Algorithm)
				}
			}
		}
	}
	return plugin, nil
}

func newIndex(version int, trustedKeys []TrustedKey, plugins []Plugin) (*Index, error) {
	if version == 0 {
		return nil, oops.In("bu1ld.plugin_registry").New("plugin registry version is required")
	}
	items := list.NewList(plugins...).Values()
	slices.SortFunc(items, func(left, right Plugin) int {
		return strings.Compare(left.ID, right.ID)
	})
	return &Index{Version: version, TrustedKeys: trustedKeys, Items: items}, nil
}

func registrySignatureAlgorithmSupported(algorithm string) bool {
	algorithm = strings.TrimSpace(strings.ToLower(algorithm))
	return algorithm == "" || algorithm == "ed25519"
}

func (p Plugin) matches(query string) bool {
	values := list.NewList(
		p.ID,
		p.Namespace,
		p.Owner,
		p.Description,
		p.Homepage,
		strings.Join(p.Tags, " "),
	)
	return values.AnyMatch(func(_ int, value string) bool {
		return strings.Contains(strings.ToLower(value), query)
	})
}

func registryPath(source string) (string, error) {
	if strings.HasPrefix(source, "file://") {
		path, err := fileURLPath(source)
		if err != nil {
			return "", err
		}
		info, statErr := os.Stat(path)
		if statErr == nil && info.IsDir() {
			return filepath.Join(path, defaultIndexFile), nil
		}
		return path, nil
	}
	info, err := os.Stat(source)
	if err == nil && info.IsDir() {
		return filepath.Join(source, defaultIndexFile), nil
	}
	if err == nil {
		return source, nil
	}
	if os.IsNotExist(err) && filepath.Ext(source) == "" {
		return filepath.Join(source, defaultIndexFile), nil
	}
	return source, nil
}

func registryURL(source string) (string, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return "", oops.In("bu1ld.plugin_registry").
			With("url", source).
			Wrapf(err, "parse plugin registry url")
	}
	if strings.HasSuffix(parsed.Path, "/") {
		parsed.Path = path.Join(parsed.Path, defaultIndexFile)
	} else if path.Ext(parsed.Path) == "" {
		parsed.Path = path.Join(parsed.Path, defaultIndexFile)
	}
	return parsed.String(), nil
}

func resolveRegistryRef(base, ref string) (string, error) {
	if isHTTPURL(ref) || strings.HasPrefix(ref, "file://") || filepath.IsAbs(ref) {
		return ref, nil
	}
	if isHTTPURL(base) {
		baseURL, err := url.Parse(base)
		if err != nil {
			return "", oops.In("bu1ld.plugin_registry").
				With("url", base).
				Wrapf(err, "parse plugin registry url")
		}
		refURL, err := url.Parse(ref)
		if err != nil {
			return "", oops.In("bu1ld.plugin_registry").
				With("url", ref).
				Wrapf(err, "parse plugin registry entry url")
		}
		return baseURL.ResolveReference(refURL).String(), nil
	}
	if strings.HasPrefix(base, "file://") {
		path, err := fileURLPath(base)
		if err != nil {
			return "", err
		}
		return filepath.Join(filepath.Dir(path), filepath.FromSlash(ref)), nil
	}
	if filepath.IsAbs(ref) {
		return ref, nil
	}
	return filepath.Join(filepath.Dir(base), filepath.FromSlash(ref)), nil
}

func resolvePluginAssets(plugin *Plugin, base string) {
	for versionIndex := range plugin.Versions {
		version := &plugin.Versions[versionIndex]
		for assetIndex := range version.Assets {
			asset := &version.Assets[assetIndex]
			asset.URL = resolveAssetURL(base, asset.URL)
			for signatureIndex := range asset.Signatures {
				signature := &asset.Signatures[signatureIndex]
				signature.URL = resolveAssetURL(base, signature.URL)
			}
		}
	}
}

func validateTrustedKeys(path string, keys []TrustedKey) error {
	seen := map[string]struct{}{}
	for _, key := range keys {
		id := strings.TrimSpace(key.ID)
		if id == "" {
			return oops.In("bu1ld.plugin_registry").
				With("file", path).
				New("registry trusted key id is required")
		}
		if _, ok := seen[id]; ok {
			return oops.In("bu1ld.plugin_registry").
				With("file", path).
				With("key", id).
				New("registry trusted key id is duplicated")
		}
		seen[id] = struct{}{}
		algorithm := strings.TrimSpace(strings.ToLower(key.Algorithm))
		if algorithm != "" && algorithm != "ed25519" {
			return oops.In("bu1ld.plugin_registry").
				With("file", path).
				With("key", id).
				Errorf("registry trusted key algorithm %q is unsupported", key.Algorithm)
		}
		if strings.TrimSpace(key.PublicKey) == "" {
			return oops.In("bu1ld.plugin_registry").
				With("file", path).
				With("key", id).
				New("registry trusted key public key is required")
		}
	}
	return nil
}

func resolveAssetURL(base, assetURL string) string {
	if assetURL == "" || isHTTPURL(assetURL) || strings.HasPrefix(assetURL, "file://") || filepath.IsAbs(assetURL) {
		return assetURL
	}
	if isHTTPURL(base) {
		baseURL, err := url.Parse(base)
		if err != nil {
			return assetURL
		}
		refURL, err := url.Parse(assetURL)
		if err != nil {
			return assetURL
		}
		return baseURL.ResolveReference(refURL).String()
	}
	if strings.HasPrefix(base, "file://") {
		path, err := fileURLPath(base)
		if err != nil {
			return assetURL
		}
		return filepath.Join(filepath.Dir(path), filepath.FromSlash(assetURL))
	}
	return filepath.Join(filepath.Dir(base), filepath.FromSlash(assetURL))
}

func isHTTPURL(value string) bool {
	return strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://")
}

func fileURLPath(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", oops.In("bu1ld.plugin_registry").
			With("url", value).
			Wrapf(err, "parse file url")
	}
	path := parsed.Path
	if parsed.Host != "" {
		path = "//" + parsed.Host + path
	}
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path), nil
}
