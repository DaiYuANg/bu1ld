package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
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
	items := append([]LockedPlugin(nil), plugins...)
	sort.SliceStable(items, func(i, j int) bool {
		left := lockKey(items[i])
		right := lockKey(items[j])
		return left < right
	})
	return LockFile{
		Version: 1,
		Plugins: items,
	}
}

func ReadLockFile(path string) (LockFile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return LockFile{}, false, nil
		}
		return LockFile{}, false, err
	}
	var lock LockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		return LockFile{}, true, fmt.Errorf("parse %s: %w", path, err)
	}
	if lock.Version == 0 {
		return LockFile{}, true, fmt.Errorf("validate %s: lockfile version is required", path)
	}
	return lock, true, nil
}

func WriteLockFile(path string, lock LockFile) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (l LockFile) Find(source Source, namespace string, id string) (LockedPlugin, bool) {
	for _, plugin := range l.Plugins {
		if plugin.Source == source && plugin.Namespace == namespace && plugin.ID == id {
			return plugin, true
		}
	}
	return LockedPlugin{}, false
}

func ChecksumFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func lockKey(plugin LockedPlugin) string {
	return string(plugin.Source) + "\x00" + plugin.Namespace + "\x00" + plugin.ID + "\x00" + plugin.Version + "\x00" + plugin.Path
}
