package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"bu1ld/internal/fsx"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/set"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/spf13/afero"
)

type File struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Mode   int64  `json:"mode"`
	Size   int64  `json:"size"`
}

type Snapshotter struct {
	fs afero.Fs
}

func NewSnapshotter(fs afero.Fs) *Snapshotter {
	return &Snapshotter{fs: fs}
}

func (s *Snapshotter) Inputs(root string, patterns []string) ([]File, error) {
	matched := set.NewOrderedSet[string]()

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		paths, err := s.match(root, pattern)
		if err != nil {
			return nil, err
		}
		matched.Add(paths...)
	}

	files := matched.Values()
	slices.Sort(files)

	snapshots := list.NewList[File]()
	for _, file := range files {
		item, err := s.File(root, file)
		if err != nil {
			return nil, err
		}
		snapshots.Add(item)
	}

	return snapshots.Values(), nil
}

func (s *Snapshotter) File(root string, relativePath string) (File, error) {
	absolutePath := filepath.Join(root, filepath.FromSlash(relativePath))
	info, err := s.fs.Stat(absolutePath)
	if err != nil {
		return File{}, fmt.Errorf("stat %s: %w", absolutePath, err)
	}
	if info.IsDir() {
		return File{}, fmt.Errorf("%s is a directory", relativePath)
	}

	digest, err := DigestFile(s.fs, absolutePath)
	if err != nil {
		return File{}, fmt.Errorf("hash %s: %w", absolutePath, err)
	}

	return File{
		Path:   filepath.ToSlash(relativePath),
		Digest: digest,
		Mode:   int64(info.Mode().Perm()),
		Size:   info.Size(),
	}, nil
}

func (s *Snapshotter) match(root string, pattern string) ([]string, error) {
	if hasGlob(pattern) {
		absolutePattern := filepath.Join(root, filepath.FromSlash(pattern))
		matches, err := fsx.Glob(s.fs, absolutePattern, doublestar.WithFilesOnly(), doublestar.WithNoFollow())
		if err != nil {
			return nil, fmt.Errorf("match %q: %w", pattern, err)
		}
		return relativeFiles(s.fs, root, matches)
	}

	absolutePath := filepath.Join(root, filepath.FromSlash(pattern))
	info, err := s.fs.Stat(absolutePath)
	if err != nil {
		if isNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat %s: %w", absolutePath, err)
	}
	if !info.IsDir() {
		return []string{filepath.ToSlash(pattern)}, nil
	}

	files := list.NewList[string]()
	err = fsx.WalkFiles(s.fs, absolutePath, func(path string, _ os.FileInfo) error {
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("resolve relative path %s: %w", path, relErr)
		}
		files.Add(filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", absolutePath, err)
	}

	values := files.Values()
	slices.Sort(values)
	return values, nil
}

func DigestFile(fs afero.Fs, path string) (result string, err error) {
	file, err := fs.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close %s: %w", path, closeErr)
		}
	}()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func HashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func hasGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func relativeFiles(fs afero.Fs, root string, paths []string) ([]string, error) {
	files := list.NewList[string]()
	for _, path := range paths {
		info, err := fs.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
		if info.IsDir() {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, fmt.Errorf("resolve relative path %s: %w", path, err)
		}
		files.Add(filepath.ToSlash(rel))
	}
	values := files.Values()
	slices.Sort(values)
	return values, nil
}

func isNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}
