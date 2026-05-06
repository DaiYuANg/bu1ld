package dsl

import (
	"path/filepath"

	"github.com/samber/oops"
)

func cleanAbsPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", oops.In("bu1ld.dsl").
			With("path", path).
			Wrapf(err, "resolve absolute path")
	}
	return filepath.Clean(absPath), nil
}
