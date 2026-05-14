package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/lyonbrown4d/bu1ld/internal/build"

	"github.com/fxamacker/cbor/v2"
	"github.com/spf13/afero"
)

func copyFile(fs afero.Fs, src, dst string, mode os.FileMode) (err error) {
	in, err := fs.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() {
		if closeErr := in.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close %s: %w", src, closeErr)
		}
	}()

	out, err := fs.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open %s: %w", dst, err)
	}
	defer func() {
		if closeErr := out.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close %s: %w", dst, closeErr)
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	return nil
}

func atomicWriteFile(fs afero.Fs, path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := fs.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent directory %s: %w", dir, err)
	}
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.%d.tmp", filepath.Base(path), time.Now().UnixNano()))
	if err := afero.WriteFile(fs, tmp, data, mode); err != nil {
		return fmt.Errorf("write temp file %s: %w", tmp, err)
	}
	if err := fs.Rename(tmp, path); err != nil {
		_ = fs.Remove(path)
		if renameErr := fs.Rename(tmp, path); renameErr != nil {
			_ = fs.Remove(tmp)
			return fmt.Errorf("replace %s: %w", path, renameErr)
		}
	}
	return nil
}

var outputFilesDigestEncMode = mustOutputFilesDigestEncMode()

func mustOutputFilesDigestEncMode() cbor.EncMode {
	mode, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		panic(err)
	}
	return mode
}

func isNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}

func (s *Store) taskAbsolute(task build.Task, path string) string {
	return s.absolute(s.taskRelative(task, path))
}

func (s *Store) taskRelative(task build.Task, path string) string {
	if filepath.IsAbs(path) || task.WorkDir == "" {
		return path
	}
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(task.WorkDir), filepath.FromSlash(path)))
}
