package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DaiYuANg/arcgo/collectionx"
	"github.com/bmatcuk/doublestar/v4"
)

type File struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Mode   int64  `json:"mode"`
	Size   int64  `json:"size"`
}

type Snapshotter struct{}

func NewSnapshotter() *Snapshotter {
	return &Snapshotter{}
}

func (s *Snapshotter) Inputs(root string, patterns []string) ([]File, error) {
	matched := collectionx.NewOrderedSet[string]()

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
	sort.Strings(files)

	snapshots := collectionx.NewList[File]()
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
	info, err := os.Stat(absolutePath)
	if err != nil {
		return File{}, err
	}
	if info.IsDir() {
		return File{}, fmt.Errorf("%s is a directory", relativePath)
	}

	digest, err := DigestFile(absolutePath)
	if err != nil {
		return File{}, err
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
		matches, err := doublestar.FilepathGlob(absolutePattern, doublestar.WithFilesOnly(), doublestar.WithNoFollow())
		if err != nil {
			return nil, fmt.Errorf("match %q: %w", pattern, err)
		}
		return relativeFiles(root, matches)
	}

	absolutePath := filepath.Join(root, filepath.FromSlash(pattern))
	info, err := os.Stat(absolutePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return []string{filepath.ToSlash(pattern)}, nil
	}

	files := collectionx.NewList[string]()
	err = filepath.WalkDir(absolutePath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files.Add(filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}

	values := files.Values()
	sort.Strings(values)
	return values, nil
}

func DigestFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
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

func relativeFiles(root string, paths []string) ([]string, error) {
	files := collectionx.NewList[string]()
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		files.Add(filepath.ToSlash(rel))
	}
	values := files.Values()
	sort.Strings(values)
	return values, nil
}
