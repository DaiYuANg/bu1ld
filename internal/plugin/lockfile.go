package plugin

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	iofs "io/fs"
	"slices"

	"github.com/arcgolabs/collectionx/list"
	"github.com/samber/mo"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

const LockFileName = "bu1ld.lock"

type LockFile struct {
	Version int            `json:"version"`
	Plugins []LockedPlugin `json:"plugins"`
}

type LockedPlugin struct {
	Source    Source `json:"source"`
	Namespace string `json:"namespace,omitempty"`
	ID        string `json:"id"`
	Version   string `json:"version,omitempty"`
	Path      string `json:"path,omitempty"`
	Checksum  string `json:"checksum,omitempty"`
}

func NewLockFile(plugins []LockedPlugin) LockFile {
	items := slices.Clone(plugins)
	slices.SortFunc(items, func(left, right LockedPlugin) int {
		return cmp.Compare(lockKey(left), lockKey(right))
	})
	return LockFile{
		Version: 1,
		Plugins: items,
	}
}

func ReadLockFile(path string) (LockFile, bool, error) {
	fs := afero.NewOsFs()
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		if isNotExist(err) {
			return LockFile{}, false, nil
		}
		return LockFile{}, false, oops.In("bu1ld.plugins").
			With("file", path).
			Wrapf(err, "read plugin lockfile")
	}
	var lock LockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		return LockFile{}, true, oops.In("bu1ld.plugins").
			With("file", path).
			Wrapf(err, "parse plugin lockfile")
	}
	if lock.Version == 0 {
		return LockFile{}, true, oops.In("bu1ld.plugins").
			With("file", path).
			New("plugin lockfile version is required")
	}
	return lock, true, nil
}

func WriteLockFile(path string, lock LockFile) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return oops.In("bu1ld.plugins").
			With("file", path).
			Wrapf(err, "marshal plugin lockfile")
	}
	data = append(data, '\n')
	if err := afero.WriteFile(afero.NewOsFs(), path, data, 0o600); err != nil {
		return oops.In("bu1ld.plugins").
			With("file", path).
			Wrapf(err, "write plugin lockfile")
	}
	return nil
}

func (l LockFile) Find(source Source, namespace, id string) (LockedPlugin, bool) {
	return l.FindOption(source, namespace, id).Get()
}

func (l LockFile) FindOption(source Source, namespace, id string) mo.Option[LockedPlugin] {
	return list.NewList(l.Plugins...).FirstWhere(func(_ int, plugin LockedPlugin) bool {
		if plugin.Source == source && plugin.Namespace == namespace && plugin.ID == id {
			return true
		}
		return false
	})
}

func ChecksumFile(path string) (result string, err error) {
	file, err := afero.NewOsFs().Open(path)
	if err != nil {
		return "", oops.In("bu1ld.plugins").
			With("file", path).
			Wrapf(err, "open plugin file")
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = oops.In("bu1ld.plugins").
				With("file", path).
				Wrapf(closeErr, "close plugin file")
		}
	}()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", oops.In("bu1ld.plugins").
			With("file", path).
			Wrapf(err, "hash plugin file")
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func lockKey(plugin LockedPlugin) string {
	return string(plugin.Source) + "\x00" + plugin.Namespace + "\x00" + plugin.ID + "\x00" + plugin.Version + "\x00" + plugin.Path
}

func isNotExist(err error) bool {
	return err != nil && errors.Is(err, iofs.ErrNotExist)
}
