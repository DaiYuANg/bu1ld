package app

import (
	"os"
	"path/filepath"

	"github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

type initFile struct {
	Path    string
	Content string
	Mode    os.FileMode
}

func (a *App) initProject() error {
	files := starterFiles(a.loader.BuildFilePath(), a.loader.LoadOptions().ProjectDir)
	if !a.request.ForceWrite {
		existing := existingStarterFiles(a.loader.FS(), files)
		if existing.Len() > 0 {
			return oops.In("bu1ld.init").
				With("files", existing.Values()).
				New("starter files already exist; rerun with --force to overwrite")
		}
	}

	for _, file := range files.Values() {
		if err := writeStarterFile(a.loader.FS(), file); err != nil {
			return err
		}
	}
	return writef(a.output, "initialized bu1ld project in %s\n", a.loader.LoadOptions().ProjectDir)
}

func starterFiles(buildFilePath, projectDir string) *list.List[initFile] {
	return list.NewList[initFile](
		initFile{
			Path: filepath.Clean(buildFilePath),
			Content: `workspace {
  name = "starter"
  default = build
}

import "tasks/*.bu1ld"
`,
			Mode: 0o644,
		},
		initFile{
			Path: filepath.Join(projectDir, "tasks", "archive.bu1ld"),
			Content: `archive.zip package_zip {
  srcs = ["src/**"]
  out = "dist/source.zip"
}

task build {
  deps = [package_zip]
  command = []
}
`,
			Mode: 0o644,
		},
		initFile{
			Path: filepath.Join(projectDir, "src", "message.txt"),
			Content: `hello from bu1ld
`,
			Mode: 0o644,
		},
	)
}

func existingStarterFiles(fs afero.Fs, files *list.List[initFile]) *list.List[string] {
	existing := list.NewList[string]()
	files.Range(func(_ int, file initFile) bool {
		found, err := afero.Exists(fs, file.Path)
		if err == nil && found {
			existing.Add(file.Path)
		}
		return true
	})
	return existing
}

func writeStarterFile(fs afero.Fs, file initFile) error {
	if err := fs.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
		return oops.In("bu1ld.init").
			With("dir", filepath.Dir(file.Path)).
			Wrapf(err, "create starter directory")
	}
	if err := afero.WriteFile(fs, file.Path, []byte(file.Content), file.Mode); err != nil {
		return oops.In("bu1ld.init").
			With("file", file.Path).
			Wrapf(err, "write starter file")
	}
	return nil
}
