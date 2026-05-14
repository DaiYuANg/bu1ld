package app

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/lyonbrown4d/bu1ld/internal/build"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/set"
	"github.com/samber/oops"
	"golang.org/x/sys/execabs"
)

func (a *App) printAffected(ctx context.Context, project build.Project) error {
	files, err := changedFiles(ctx, a.loader.LoadOptions().ProjectDir, a.request.BaseRef)
	if err != nil {
		return err
	}
	packages := affectedPackages(project, files)
	for _, name := range packages {
		if err := writeLine(a.output, name); err != nil {
			return err
		}
	}
	return nil
}

func changedFiles(ctx context.Context, projectDir, baseRef string) ([]string, error) {
	if baseRef == "" {
		baseRef = "HEAD"
	}
	cmd, err := changedFilesCommand(ctx, baseRef)
	if err != nil {
		return nil, err
	}
	cmd.Dir = projectDir
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, oops.In("bu1ld.affected").
			With("base", baseRef).
			With("stderr", strings.TrimSpace(stderr.String())).
			Wrapf(err, "list changed files")
	}
	files := list.NewList[string]()
	for line := range strings.SplitSeq(out.String(), "\n") {
		file := strings.TrimSpace(line)
		if file != "" {
			files.Add(filepath.ToSlash(file))
		}
	}
	return files.Values(), nil
}

func changedFilesCommand(ctx context.Context, baseRef string) (*exec.Cmd, error) {
	switch baseRef {
	case "HEAD":
		return execabs.CommandContext(ctx, "git", "diff", "--name-only", "HEAD", "--"), nil
	default:
		return nil, oops.In("bu1ld.affected").
			With("base", baseRef).
			New("custom affected base refs are not supported by the hardened git runner yet")
	}
}

func affectedPackages(project build.Project, files []string) []string {
	direct := set.NewSet[string]()
	if project.Packages != nil {
		project.Packages.Range(func(_ int, pkg build.Package) bool {
			for _, file := range files {
				if fileInPackage(file, pkg.Dir) {
					direct.Add(pkg.Name)
					return true
				}
			}
			return true
		})
	}
	affected := includeDependentPackages(project, direct)
	values := affected.Values()
	slices.Sort(values)
	return values
}

func includeDependentPackages(project build.Project, direct *set.Set[string]) *set.Set[string] {
	affected := set.NewSet[string](direct.Values()...)
	changed := true
	for changed {
		changed = false
		if project.Packages == nil {
			continue
		}
		project.Packages.Range(func(_ int, pkg build.Package) bool {
			if affected.Contains(pkg.Name) {
				return true
			}
			if list.NewList(build.Values(pkg.Deps)...).AnyMatch(func(_ int, dep string) bool {
				return affected.Contains(dep)
			}) {
				affected.Add(pkg.Name)
				changed = true
				return true
			}
			return true
		})
	}
	return affected
}

func fileInPackage(file, packageDir string) bool {
	file = filepath.ToSlash(file)
	packageDir = strings.Trim(filepath.ToSlash(packageDir), "/")
	return file == packageDir || strings.HasPrefix(file, packageDir+"/")
}
