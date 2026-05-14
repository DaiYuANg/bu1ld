// Package archive provides built-in archive actions.
package archive

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	pluginparams "github.com/lyonbrown4d/bu1ld/internal/plugins/params"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/set"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/mholt/archives"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

type Handler struct {
	kind string
}

func NewZipHandler() *Handler {
	return &Handler{kind: ZipActionKind}
}

func NewTarHandler() *Handler {
	return &Handler{kind: TarActionKind}
}

func (h *Handler) Kind() string {
	return h.kind
}

func (h *Handler) Run(ctx context.Context, workDir string, params map[string]any, _ io.Writer) error {
	spec := archiveSpecFromParams(params)
	files, err := expandSourceFiles(workDir, spec.Srcs)
	if err != nil {
		return err
	}
	switch h.kind {
	case ZipActionKind:
		return writeZip(ctx, workDir, spec.Out, files)
	case TarActionKind:
		return writeTar(ctx, workDir, spec.Out, spec.Gzip, files)
	default:
		return oops.In("bu1ld.archive").
			With("action", h.kind).
			Errorf("unknown archive action %q", h.kind)
	}
}

type archiveSpec struct {
	Srcs []string
	Out  string
	Gzip bool
}

func archiveSpecFromParams(params map[string]any) archiveSpec {
	return archiveSpec{
		Srcs: pluginparams.StringSlice(params, "srcs"),
		Out:  pluginparams.String(params, "out"),
		Gzip: pluginparams.Bool(params, "gzip"),
	}
}

func expandSourceFiles(workDir string, srcs []string) ([]string, error) {
	files := set.NewOrderedSet[string]()
	for _, src := range srcs {
		matches, err := matchSource(workDir, src)
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			rel := filepath.ToSlash(match)
			files.Add(rel)
		}
	}
	return files.Values(), nil
}

func matchSource(workDir, src string) ([]string, error) {
	if hasGlob(src) {
		matches, err := doublestar.Glob(os.DirFS(workDir), filepath.ToSlash(src), doublestar.WithFilesOnly(), doublestar.WithNoFollow())
		if err != nil {
			return nil, oops.In("bu1ld.archive").
				With("src", src).
				Wrapf(err, "match archive source glob")
		}
		return matches, nil
	}

	path := filepath.Join(workDir, src)
	info, err := os.Stat(path)
	if err != nil {
		return nil, oops.In("bu1ld.archive").
			With("src", src).
			Wrapf(err, "stat archive source")
	}
	if !info.IsDir() {
		return []string{src}, nil
	}

	files := list.NewList[string]()
	err = filepath.WalkDir(path, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(workDir, filePath)
		if relErr != nil {
			return oops.In("bu1ld.archive").
				With("src", src).
				With("path", filePath).
				Wrapf(relErr, "relativize archive source")
		}
		files.Add(rel)
		return nil
	})
	if err != nil {
		return nil, oops.In("bu1ld.archive").
			With("src", src).
			Wrapf(err, "walk archive source directory")
	}
	return files.Values(), nil
}

func writeZip(ctx context.Context, workDir, out string, files []string) error {
	return writeArchive(ctx, workDir, out, files, archives.Zip{})
}

func writeTar(ctx context.Context, workDir, out string, gzipEnabled bool, files []string) error {
	format := archives.Archiver(archives.Tar{})
	if gzipEnabled {
		format = archives.CompressedArchive{
			Compression: archives.Gz{},
			Archival:    archives.Tar{},
		}
	}
	return writeArchive(ctx, workDir, out, files, format)
}

func writeArchive(ctx context.Context, workDir, out string, files []string, format archives.Archiver) (err error) {
	target, err := createOutput(workDir, out)
	if err != nil {
		return err
	}
	defer func() {
		err = closeArchive(err, target.Close(), "close archive output")
	}()

	archiveFiles, err := archiveFilesFromDisk(ctx, workDir, files)
	if err != nil {
		return err
	}
	return format.Archive(ctx, target, archiveFiles)
}

func archiveFilesFromDisk(ctx context.Context, workDir string, files []string) ([]archives.FileInfo, error) {
	archiveFiles := list.NewListWithCapacity[archives.FileInfo](len(files))
	for _, file := range files {
		source := filepath.Join(workDir, filepath.FromSlash(file))
		items, err := archives.FilesFromDisk(ctx, nil, map[string]string{
			source: filepath.ToSlash(file),
		})
		if err != nil {
			return nil, oops.In("bu1ld.archive").
				With("file", file).
				Wrapf(err, "read archive source")
		}
		archiveFiles.Add(items...)
	}
	return archiveFiles.Values(), nil
}

func createOutput(workDir, out string) (afero.File, error) {
	path := filepath.Join(workDir, out)
	fs := afero.NewOsFs()
	if err := fs.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, oops.In("bu1ld.archive").With("out", out).Wrapf(err, "create archive output directory")
	}
	file, err := fs.Create(path)
	if err != nil {
		return nil, oops.In("bu1ld.archive").With("out", out).Wrapf(err, "create archive output")
	}
	return file, nil
}

func closeArchive(err, closeErr error, message string) error {
	if closeErr == nil {
		return err
	}
	wrapped := oops.In("bu1ld.archive").Wrapf(closeErr, "%s", message)
	if err != nil {
		return errors.Join(err, wrapped)
	}
	return wrapped
}

func hasGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}
