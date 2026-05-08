package cache

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"

	"bu1ld/internal/build"
	"bu1ld/internal/cachefile"
	"bu1ld/internal/config"
	"bu1ld/internal/fsx"
	"bu1ld/internal/snapshot"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/set"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

type Store struct {
	cfg    config.Config
	fs     afero.Fs
	remote *RemoteClient
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
	Mode   uint32       `json:"mode,omitempty"`
	Files  []OutputFile `json:"files,omitempty"`
}

type OutputFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Mode   uint32 `json:"mode"`
}

func NewStore(cfg config.Config, fs afero.Fs) *Store {
	store := &Store{
		cfg: cfg,
		fs:  fs,
	}
	if cfg.RemoteCacheURL != "" {
		store.remote = NewRemoteClient(cfg.RemoteCacheURL)
	}
	return store
}

func (s *Store) Load(actionKey string) (Record, bool, error) {
	record, hit, err := s.loadLocal(actionKey)
	if err != nil || hit {
		return record, hit, err
	}
	if s.remote == nil || !s.cfg.RemoteCachePull {
		return Record{}, false, nil
	}
	record, hit, err = s.loadRemote(actionKey)
	if err != nil || !hit {
		return Record{}, hit, err
	}
	return record, true, nil
}

func (s *Store) Save(task build.Task, actionKey string) error {
	record := Record{
		TaskName:  task.Name,
		ActionKey: actionKey,
	}

	outputs := list.NewList[OutputRecord]()
	for _, output := range build.Values(task.Outputs) {
		entry, err := s.captureOutput(task, output)
		if err != nil {
			return oops.In("bu1ld.cache").
				With("task", task.Name).
				With("action_key", actionKey).
				With("output", output).
				Wrapf(err, "capture task output")
		}
		outputs.Add(entry)
	}
	record.Outputs = outputs.Values()
	path := s.recordPath(actionKey)
	if err := cachefile.Write(s.fs, path, record); err != nil {
		return oops.In("bu1ld.cache").
			With("task", task.Name).
			With("action_key", actionKey).
			With("path", path).
			Wrapf(err, "write action cache record")
	}
	if s.remote != nil && s.cfg.RemoteCachePush {
		if err := s.pushRemote(record); err != nil {
			return oops.In("bu1ld.cache").
				With("task", task.Name).
				With("action_key", actionKey).
				Wrapf(err, "push remote action cache record")
		}
	}
	return nil
}

func (s *Store) loadLocal(actionKey string) (Record, bool, error) {
	path := s.recordPath(actionKey)
	if _, err := s.fs.Stat(path); err != nil {
		if isNotExist(err) {
			return Record{}, false, nil
		}
		return Record{}, false, oops.In("bu1ld.cache").
			With("action_key", actionKey).
			With("path", path).
			Wrapf(err, "stat action cache record")
	}

	var record Record
	if err := cachefile.Read(s.fs, path, &record); err != nil {
		return Record{}, false, oops.In("bu1ld.cache").
			With("action_key", actionKey).
			With("path", path).
			Wrapf(err, "read action cache record")
	}
	return record, true, nil
}

func (s *Store) loadRemote(actionKey string) (Record, bool, error) {
	data, hit, err := s.remote.GetAction(actionKey)
	if err != nil || !hit {
		return Record{}, hit, err
	}
	var record Record
	if err := cachefile.Unmarshal(data, &record); err != nil {
		return Record{}, false, oops.In("bu1ld.cache").
			With("action_key", actionKey).
			Wrapf(err, "decode remote action cache record")
	}
	if record.ActionKey != "" && record.ActionKey != actionKey {
		return Record{}, false, oops.In("bu1ld.cache").
			With("action_key", actionKey).
			With("record_action_key", record.ActionKey).
			New("remote action cache record key mismatch")
	}
	if err := s.ensureRemoteBlobs(record); err != nil {
		return Record{}, false, err
	}
	if err := s.writeRecordBytes(actionKey, data); err != nil {
		return Record{}, false, err
	}
	return record, true, nil
}

func (s *Store) ensureRemoteBlobs(record Record) error {
	for _, digest := range recordDigests(record) {
		if _, err := s.fs.Stat(s.blobPath(digest)); err == nil {
			continue
		} else if !isNotExist(err) {
			return oops.In("bu1ld.cache").
				With("digest", digest).
				Wrapf(err, "stat local cache blob")
		}
		data, hit, err := s.remote.GetBlob(digest)
		if err != nil {
			return oops.In("bu1ld.cache").
				With("digest", digest).
				Wrapf(err, "download remote cache blob")
		}
		if !hit {
			return oops.In("bu1ld.cache").
				With("digest", digest).
				New("remote action cache blob is missing")
		}
		if snapshot.HashBytes(data) != digest {
			return oops.In("bu1ld.cache").
				With("digest", digest).
				New("remote cache blob digest mismatch")
		}
		if err := s.writeBlobBytes(digest, data); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) pushRemote(record Record) error {
	for _, digest := range recordDigests(record) {
		exists, err := s.remote.HasBlob(digest)
		if err != nil {
			return oops.In("bu1ld.cache").
				With("digest", digest).
				Wrapf(err, "check remote cache blob")
		}
		if exists {
			continue
		}
		data, err := afero.ReadFile(s.fs, s.blobPath(digest))
		if err != nil {
			return oops.In("bu1ld.cache").
				With("digest", digest).
				Wrapf(err, "read local cache blob")
		}
		if err := s.remote.PutBlob(digest, data); err != nil {
			return oops.In("bu1ld.cache").
				With("digest", digest).
				Wrapf(err, "upload remote cache blob")
		}
	}
	data, err := afero.ReadFile(s.fs, s.recordPath(record.ActionKey))
	if err != nil {
		return oops.In("bu1ld.cache").
			With("action_key", record.ActionKey).
			Wrapf(err, "read local action cache record")
	}
	if err := s.remote.PutAction(record.ActionKey, data); err != nil {
		return oops.In("bu1ld.cache").
			With("action_key", record.ActionKey).
			Wrapf(err, "upload remote action cache record")
	}
	return nil
}

func (s *Store) Restore(record Record) error {
	for _, output := range record.Outputs {
		switch output.Kind {
		case "file":
			if err := s.restoreFile(output.Path, output.Digest, os.FileMode(output.Mode)); err != nil {
				return oops.In("bu1ld.cache").
					With("path", output.Path).
					With("digest", output.Digest).
					Wrapf(err, "restore cached file")
			}
		case "dir":
			if err := s.fs.MkdirAll(s.absolute(output.Path), 0o755); err != nil {
				return oops.In("bu1ld.cache").
					With("path", output.Path).
					Wrapf(err, "prepare cached output directory")
			}
			for _, file := range output.Files {
				rel := filepath.Join(output.Path, filepath.FromSlash(file.Path))
				if err := s.restoreFile(rel, file.Digest, os.FileMode(file.Mode)); err != nil {
					return oops.In("bu1ld.cache").
						With("path", output.Path).
						With("file", file.Path).
						With("digest", file.Digest).
						Wrapf(err, "restore cached output file")
				}
			}
		default:
			return oops.In("bu1ld.cache").
				With("path", output.Path).
				With("kind", output.Kind).
				Errorf("unknown cached output kind %q", output.Kind)
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
		if _, err := s.fs.Stat(s.taskAbsolute(task, output)); err != nil {
			return false
		}
	}
	return true
}

func (s *Store) Clean() error {
	if err := s.fs.RemoveAll(s.cfg.CachePath()); err != nil {
		return oops.In("bu1ld.cache").
			With("path", s.cfg.CachePath()).
			Wrapf(err, "remove cache directory")
	}
	return nil
}

func (s *Store) writeRecordBytes(actionKey string, data []byte) error {
	path := s.recordPath(actionKey)
	if err := s.fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return oops.In("bu1ld.cache").
			With("path", filepath.Dir(path)).
			Wrapf(err, "create action cache directory")
	}
	if err := afero.WriteFile(s.fs, path, data, 0o644); err != nil {
		return oops.In("bu1ld.cache").
			With("action_key", actionKey).
			With("path", path).
			Wrapf(err, "write action cache record")
	}
	return nil
}

func (s *Store) writeBlobBytes(digest string, data []byte) error {
	path := s.blobPath(digest)
	if err := s.fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return oops.In("bu1ld.cache").
			With("path", filepath.Dir(path)).
			Wrapf(err, "create blob cache directory")
	}
	if err := afero.WriteFile(s.fs, path, data, 0o644); err != nil {
		return oops.In("bu1ld.cache").
			With("digest", digest).
			With("path", path).
			Wrapf(err, "write cache blob")
	}
	return nil
}

func recordDigests(record Record) []string {
	seen := set.NewSet[string]()
	for _, output := range record.Outputs {
		if output.Kind == "file" && output.Digest != "" {
			seen.Add(output.Digest)
			continue
		}
		for _, file := range output.Files {
			if file.Digest != "" {
				seen.Add(file.Digest)
			}
		}
	}

	digests := seen.Values()
	slices.Sort(digests)
	return digests
}

func (s *Store) captureOutput(task build.Task, output string) (OutputRecord, error) {
	rootRelativeOutput := s.taskRelative(task, output)
	path := s.absolute(rootRelativeOutput)
	info, err := s.fs.Stat(path)
	if err != nil {
		return OutputRecord{}, oops.In("bu1ld.cache").
			With("output", output).
			With("path", path).
			Wrapf(err, "stat declared output")
	}

	if !info.IsDir() {
		digest, digestErr := snapshot.DigestFile(s.fs, path)
		if digestErr != nil {
			return OutputRecord{}, oops.In("bu1ld.cache").
				With("output", output).
				With("path", path).
				Wrapf(digestErr, "hash output file")
		}
		if ensureErr := s.ensureBlob(path, digest); ensureErr != nil {
			return OutputRecord{}, ensureErr
		}
		return OutputRecord{
			Path:   filepath.ToSlash(rootRelativeOutput),
			Kind:   "file",
			Digest: digest,
			Mode:   uint32(info.Mode().Perm()),
		}, nil
	}

	files := list.NewList[OutputFile]()
	err = fsx.WalkFiles(s.fs, path, func(filePath string, fileInfo os.FileInfo) error {
		rel, relErr := filepath.Rel(path, filePath)
		if relErr != nil {
			return oops.In("bu1ld.cache").
				With("output", output).
				With("path", filePath).
				Wrapf(relErr, "resolve output file path")
		}
		digest, digestErr := snapshot.DigestFile(s.fs, filePath)
		if digestErr != nil {
			return oops.In("bu1ld.cache").
				With("output", output).
				With("path", filePath).
				Wrapf(digestErr, "hash output file")
		}
		if ensureErr := s.ensureBlob(filePath, digest); ensureErr != nil {
			return ensureErr
		}
		files.Add(OutputFile{
			Path:   filepath.ToSlash(rel),
			Digest: digest,
			Mode:   uint32(fileInfo.Mode().Perm()),
		})
		return nil
	})
	if err != nil {
		return OutputRecord{}, oops.In("bu1ld.cache").
			With("output", output).
			With("path", path).
			Wrapf(err, "walk output directory")
	}

	outputFiles := files.Values()
	slices.SortFunc(outputFiles, func(left, right OutputFile) int {
		return cmp.Compare(left.Path, right.Path)
	})
	payload, err := outputFilesDigestEncMode.Marshal(outputFiles)
	if err != nil {
		return OutputRecord{}, oops.In("bu1ld.cache").
			With("output", output).
			Wrapf(err, "marshal output manifest")
	}

	return OutputRecord{
		Path:   filepath.ToSlash(rootRelativeOutput),
		Kind:   "dir",
		Digest: snapshot.HashBytes(payload),
		Files:  outputFiles,
	}, nil
}

func (s *Store) ensureBlob(path, digest string) error {
	target := s.blobPath(digest)
	if _, err := s.fs.Stat(target); err == nil {
		return nil
	}

	if err := s.fs.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return oops.In("bu1ld.cache").
			With("path", filepath.Dir(target)).
			Wrapf(err, "create blob directory")
	}
	if err := copyFile(s.fs, path, target, 0o644); err != nil {
		return oops.In("bu1ld.cache").
			With("source", path).
			With("target", target).
			Wrapf(err, "store cache blob")
	}
	return nil
}

func (s *Store) restoreFile(relativePath, digest string, mode os.FileMode) error {
	target := s.absolute(relativePath)
	if err := s.fs.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return oops.In("bu1ld.cache").
			With("path", filepath.Dir(target)).
			Wrapf(err, "create restore directory")
	}
	if err := copyFile(s.fs, s.blobPath(digest), target, mode); err != nil {
		return oops.In("bu1ld.cache").
			With("digest", digest).
			With("path", target).
			Wrapf(err, "restore cached file")
	}
	return nil
}

func (s *Store) recordPath(actionKey string) string {
	prefix := "00"
	if len(actionKey) >= 2 {
		prefix = actionKey[:2]
	}
	return filepath.Join(s.cfg.CachePath(), "actions", prefix, actionKey+".bin")
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
