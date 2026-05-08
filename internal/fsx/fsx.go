package fsx

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/arcgolabs/collectionx/list"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/spf13/afero"
)

// NewOsFS returns the process-backed filesystem used by the local runtime.
func NewOsFS() afero.Fs {
	return afero.NewOsFs()
}

// Glob matches a doublestar pattern against an afero filesystem and returns
// absolute paths using the host separator convention.
func Glob(fs afero.Fs, pattern string, opts ...doublestar.GlobOption) ([]string, error) {
	cleanPattern := filepath.ToSlash(filepath.Clean(pattern))
	base, globPattern := doublestar.SplitPattern(cleanPattern)
	baseFS := afero.NewBasePathFs(fs, filepath.FromSlash(base))

	matches, err := doublestar.Glob(afero.NewIOFS(baseFS), globPattern, opts...)
	if err != nil {
		return nil, fmt.Errorf("glob %q: %w", pattern, err)
	}

	values := list.MapList[string, string](list.NewList(matches...), func(_ int, match string) string {
		return filepath.Join(filepath.FromSlash(base), filepath.FromSlash(match))
	}).Values()
	slices.Sort(values)

	return values, nil
}

// WalkFiles visits non-directory files rooted at path.
func WalkFiles(fs afero.Fs, root string, visit func(path string, info os.FileInfo) error) error {
	if err := afero.Walk(fs, root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		return visit(path, info)
	}); err != nil {
		return fmt.Errorf("walk %s: %w", root, err)
	}
	return nil
}
