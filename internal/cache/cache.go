package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"bu1ld/internal/build"
	"bu1ld/internal/config"
	"bu1ld/internal/snapshot"

	"github.com/DaiYuANg/arcgo/collectionx"
)

type Store struct {
	cfg         config.Config
	snapshotter *snapshot.Snapshotter
}

type Record struct {
	TaskName  string         `json:"taskName"`
	ActionKey string         `json:"actionKey"`
	Outputs   []OutputRecord `json:"outputs"`
}

type OutputRecord struct {
	Path   string       `json:"path"`
	Kind   string       `json:"kind"`
	Digest string       `json:"digest"`
	Mode   int64        `json:"mode,omitempty"`
	Files  []OutputFile `json:"files,omitempty"`
}

type OutputFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Mode   int64  `json:"mode"`
}

func NewStore(cfg config.Config, snapshotter *snapshot.Snapshotter) *Store {
	return &Store{
		cfg:         cfg,
		snapshotter: snapshotter,
	}
}

func (s *Store) Load(actionKey string) (Record, bool, error) {
	data, err := os.ReadFile(s.recordPath(actionKey))
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, false, nil
		}
		return Record{}, false, err
	}

	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, false, err
	}
	return record, true, nil
}

func (s *Store) Save(task build.Task, actionKey string) error {
	record := Record{
		TaskName:  task.Name,
		ActionKey: actionKey,
	}

	for _, output := range build.Values(task.Outputs) {
		entry, err := s.captureOutput(output)
		if err != nil {
			return err
		}
		record.Outputs = append(record.Outputs, entry)
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}

	path := s.recordPath(actionKey)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) Restore(record Record) error {
	for _, output := range record.Outputs {
		switch output.Kind {
		case "file":
			if err := s.restoreFile(output.Path, output.Digest, os.FileMode(output.Mode)); err != nil {
				return err
			}
		case "dir":
			if err := os.MkdirAll(s.absolute(output.Path), 0o755); err != nil {
				return err
			}
			for _, file := range output.Files {
				rel := filepath.Join(output.Path, filepath.FromSlash(file.Path))
				if err := s.restoreFile(rel, file.Digest, os.FileMode(file.Mode)); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unknown cached output kind %q", output.Kind)
		}
	}
	return nil
}

func (s *Store) OutputsPresent(task build.Task) bool {
	outputs := build.Values(task.Outputs)
	if len(outputs) == 0 {
		return true
	}

	for _, output := range outputs {
		if _, err := os.Stat(s.absolute(output)); err != nil {
			return false
		}
	}
	return true
}

func (s *Store) Clean() error {
	return os.RemoveAll(s.cfg.CachePath())
}

func (s *Store) captureOutput(output string) (OutputRecord, error) {
	path := s.absolute(output)
	info, err := os.Stat(path)
	if err != nil {
		return OutputRecord{}, fmt.Errorf("declared output %q: %w", output, err)
	}

	if !info.IsDir() {
		digest, err := snapshot.DigestFile(path)
		if err != nil {
			return OutputRecord{}, err
		}
		if err := s.ensureBlob(path, digest); err != nil {
			return OutputRecord{}, err
		}
		return OutputRecord{
			Path:   filepath.ToSlash(output),
			Kind:   "file",
			Digest: digest,
			Mode:   int64(info.Mode().Perm()),
		}, nil
	}

	files := collectionx.NewList[OutputFile]()
	err = filepath.WalkDir(path, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(path, filePath)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		digest, err := snapshot.DigestFile(filePath)
		if err != nil {
			return err
		}
		if err := s.ensureBlob(filePath, digest); err != nil {
			return err
		}
		files.Add(OutputFile{
			Path:   filepath.ToSlash(rel),
			Digest: digest,
			Mode:   int64(info.Mode().Perm()),
		})
		return nil
	})
	if err != nil {
		return OutputRecord{}, err
	}

	outputFiles := files.Values()
	sort.Slice(outputFiles, func(i, j int) bool {
		return outputFiles[i].Path < outputFiles[j].Path
	})
	data, err := json.Marshal(outputFiles)
	if err != nil {
		return OutputRecord{}, err
	}

	return OutputRecord{
		Path:   filepath.ToSlash(output),
		Kind:   "dir",
		Digest: snapshot.HashBytes(data),
		Files:  outputFiles,
	}, nil
}

func (s *Store) ensureBlob(path string, digest string) error {
	target := s.blobPath(digest)
	if _, err := os.Stat(target); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return copyFile(path, target, 0o644)
}

func (s *Store) restoreFile(relativePath string, digest string, mode os.FileMode) error {
	target := s.absolute(relativePath)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return copyFile(s.blobPath(digest), target, mode)
}

func (s *Store) recordPath(actionKey string) string {
	prefix := "00"
	if len(actionKey) >= 2 {
		prefix = actionKey[:2]
	}
	return filepath.Join(s.cfg.CachePath(), "actions", prefix, actionKey+".json")
}

func (s *Store) blobPath(digest string) string {
	prefix := "00"
	if len(digest) >= 2 {
		prefix = digest[:2]
	}
	return filepath.Join(s.cfg.CachePath(), "blobs", prefix, digest)
}

func (s *Store) absolute(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.cfg.WorkDir, filepath.FromSlash(path))
}

func copyFile(src string, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
