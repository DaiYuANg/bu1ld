package plugin

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/arcgolabs/collectionx/list"
	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

const ManifestFileName = "plugin.toml"

type Manifest struct {
	ID        string         `toml:"id"`
	Namespace string         `toml:"namespace,omitempty"`
	Version   string         `toml:"version"`
	Binary    string         `toml:"binary"`
	Checksum  string         `toml:"checksum,omitempty"`
	Rules     []ManifestRule `toml:"rules,omitempty"`
}

type ManifestRule struct {
	Name string `toml:"name"`
}

type ManifestFile struct {
	Path     string
	Manifest Manifest
	Err      error
}

func ReadManifest(path string) (Manifest, error) {
	data, err := afero.ReadFile(afero.NewOsFs(), path)
	if err != nil {
		return Manifest{}, oops.In("bu1ld.plugins").
			With("file", path).
			Wrapf(err, "read plugin manifest")
	}
	var manifest Manifest
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, oops.In("bu1ld.plugins").
			With("file", path).
			Wrapf(err, "parse plugin manifest")
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, oops.In("bu1ld.plugins").
			With("file", path).
			Wrapf(err, "validate plugin manifest")
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.ID == "" {
		return errors.New("manifest id is required")
	}
	if m.Version == "" {
		return errors.New("manifest version is required")
	}
	if m.Binary == "" {
		return errors.New("manifest binary is required")
	}
	for _, rule := range m.Rules {
		if rule.Name == "" {
			return errors.New("manifest rule name is required")
		}
	}
	return nil
}

func (m Manifest) BinaryPath(manifestPath string) string {
	if filepath.IsAbs(m.Binary) {
		return m.Binary
	}
	return filepath.Join(filepath.Dir(manifestPath), m.Binary)
}

func ResolveManifestPath(root string, declaration Declaration) (string, bool, error) {
	path, ok, err := resolveManifestFile(root, declaration)
	if err != nil || !ok {
		return "", ok, err
	}
	manifest, err := ReadManifest(path)
	if err != nil {
		return "", false, err
	}
	if manifest.ID != pluginID(declaration) {
		return "", false, oops.In("bu1ld.plugins").
			With("file", path).
			With("plugin", pluginID(declaration)).
			With("manifest_id", manifest.ID).
			Errorf("%s id %q does not match declaration id %q", path, manifest.ID, pluginID(declaration))
	}
	if declaration.Version != "" && manifest.Version != declaration.Version {
		return "", false, oops.In("bu1ld.plugins").
			With("file", path).
			With("plugin", pluginID(declaration)).
			With("manifest_version", manifest.Version).
			With("declared_version", declaration.Version).
			Errorf("%s version %q does not match declaration version %q", path, manifest.Version, declaration.Version)
	}
	return manifest.BinaryPath(path), true, nil
}

func DiscoverManifests(root string) ([]ManifestFile, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.In("bu1ld.plugins").
			With("path", root).
			Wrapf(err, "stat plugin manifest root")
	}
	if !info.IsDir() {
		return nil, oops.In("bu1ld.plugins").
			With("path", root).
			Errorf("%s is not a directory", root)
	}

	items := list.NewList[ManifestFile]()
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() != ManifestFileName {
			return nil
		}
		manifest, manifestErr := ReadManifest(path)
		if manifestErr == nil {
			items.Add(ManifestFile{Path: path, Manifest: manifest})
			return nil
		}
		items.Add(ManifestFile{Path: path, Err: manifestErr})
		return nil
	}); err != nil {
		return nil, oops.In("bu1ld.plugins").
			With("path", root).
			Wrapf(err, "walk plugin manifests")
	}

	values := items.Values()
	slices.SortFunc(values, func(left, right ManifestFile) int {
		return cmp.Compare(left.Path, right.Path)
	})
	return values, nil
}

func resolveManifestFile(root string, declaration Declaration) (string, bool, error) {
	id := pluginID(declaration)
	if id == "" {
		return "", false, nil
	}
	if declaration.Version != "" {
		path := filepath.Join(root, id, declaration.Version, ManifestFileName)
		if fileExists(path) {
			return path, true, nil
		}
		return "", false, nil
	}

	matches, err := filepath.Glob(filepath.Join(root, id, "*", ManifestFileName))
	if err != nil {
		return "", false, oops.In("bu1ld.plugins").
			With("path", root).
			With("plugin", id).
			Wrapf(err, "glob plugin manifests")
	}
	slices.Sort(matches)
	for _, match := range matches {
		if fileExists(match) {
			return match, true, nil
		}
	}
	return "", false, nil
}
