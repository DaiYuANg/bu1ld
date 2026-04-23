package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/DaiYuANg/arcgo/collectionx"
)

const ManifestFileName = "plugin.json"

type Manifest struct {
	ID        string         `json:"id"`
	Namespace string         `json:"namespace,omitempty"`
	Version   string         `json:"version"`
	Binary    string         `json:"binary"`
	Checksum  string         `json:"checksum,omitempty"`
	Rules     []ManifestRule `json:"rules,omitempty"`
}

type ManifestRule struct {
	Name string `json:"name"`
}

type ManifestFile struct {
	Path     string
	Manifest Manifest
	Err      error
}

func ReadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("validate %s: %w", path, err)
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("manifest id is required")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest version is required")
	}
	if m.Binary == "" {
		return fmt.Errorf("manifest binary is required")
	}
	for _, rule := range m.Rules {
		if rule.Name == "" {
			return fmt.Errorf("manifest rule name is required")
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
		return "", false, fmt.Errorf("%s id %q does not match declaration id %q", path, manifest.ID, pluginID(declaration))
	}
	if declaration.Version != "" && manifest.Version != declaration.Version {
		return "", false, fmt.Errorf("%s version %q does not match declaration version %q", path, manifest.Version, declaration.Version)
	}
	return manifest.BinaryPath(path), true, nil
}

func DiscoverManifests(root string) ([]ManifestFile, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}

	items := collectionx.NewList[ManifestFile]()
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
		manifest, err := ReadManifest(path)
		if err != nil {
			items.Add(ManifestFile{Path: path, Err: err})
			return nil
		}
		items.Add(ManifestFile{Path: path, Manifest: manifest})
		return nil
	}); err != nil {
		return nil, err
	}

	values := items.Values()
	sort.SliceStable(values, func(i, j int) bool {
		return values[i].Path < values[j].Path
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
		return "", false, err
	}
	sort.Strings(matches)
	for _, match := range matches {
		if fileExists(match) {
			return match, true, nil
		}
	}
	return "", false, nil
}
