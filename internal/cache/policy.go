package cache

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/afero"
)

type PruneOptions struct {
	MaxBytes int64
	MaxAge   time.Duration
	Now      time.Time
}

type PruneResult struct {
	FilesRemoved int
	BytesRemoved int64
	BytesKept    int64
}

type cacheFileInfo struct {
	path    string
	size    int64
	modTime time.Time
}

func (s *Store) EnforcePolicy() error {
	if s.cfg.RemoteCacheMaxBytes <= 0 && s.cfg.RemoteCacheMaxAge <= 0 {
		return nil
	}
	_, err := s.Prune(PruneOptions{
		MaxBytes: s.cfg.RemoteCacheMaxBytes,
		MaxAge:   s.cfg.RemoteCacheMaxAge,
	})
	return err
}

func (s *Store) Prune(options PruneOptions) (PruneResult, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	files, err := s.cacheFiles()
	if err != nil {
		return PruneResult{}, err
	}

	result := PruneResult{}
	kept := make([]cacheFileInfo, 0, len(files))
	for _, file := range files {
		if options.MaxAge > 0 && now.Sub(file.modTime) > options.MaxAge {
			if err := s.removeCacheFile(file.path); err != nil {
				return result, err
			}
			result.FilesRemoved++
			result.BytesRemoved += file.size
			continue
		}
		kept = append(kept, file)
		result.BytesKept += file.size
	}

	if options.MaxBytes <= 0 || result.BytesKept <= options.MaxBytes {
		return result, nil
	}

	slices.SortFunc(kept, func(left, right cacheFileInfo) int {
		if value := left.modTime.Compare(right.modTime); value != 0 {
			return value
		}
		return cmp.Compare(left.path, right.path)
	})
	for _, file := range kept {
		if result.BytesKept <= options.MaxBytes {
			break
		}
		if err := s.removeCacheFile(file.path); err != nil {
			return result, err
		}
		result.FilesRemoved++
		result.BytesRemoved += file.size
		result.BytesKept -= file.size
	}
	return result, nil
}

func (s *Store) cacheFiles() ([]cacheFileInfo, error) {
	root := s.cfg.CachePath()
	files := make([]cacheFileInfo, 0)
	err := afero.Walk(s.fs, root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, cacheFileInfo{
			path:    path,
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		return nil
	})
	if err != nil && !isNotExist(err) {
		return nil, oops.In("bu1ld.cache").
			With("path", root).
			Wrapf(err, "walk cache directory")
	}
	return files, nil
}

func (s *Store) removeCacheFile(path string) error {
	if err := s.fs.Remove(path); err != nil && !isNotExist(err) {
		return oops.In("bu1ld.cache").
			With("path", path).
			Wrapf(err, "remove cache object")
	}
	removeEmptyParents(s, filepath.Dir(path))
	return nil
}

func removeEmptyParents(s *Store, dir string) {
	root := filepath.Clean(s.cfg.CachePath())
	for dir != "." && filepath.Clean(dir) != root {
		if err := s.fs.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
