package dsl

import (
	"os"
	"slices"

	"bu1ld/internal/snapshot"

	"github.com/spf13/afero"
)

func configCacheFileValid(fs afero.Fs, file configCacheFile) bool {
	if file.Path == "" || file.Checksum == "" {
		return false
	}
	checksum, err := snapshot.DigestFile(fs, file.Path)
	return err == nil && checksum == file.Checksum
}

func (l *Loader) configCacheFilesValid(files []configCacheFile) bool {
	for _, file := range files {
		if !configCacheFileValid(l.fs, file) {
			return false
		}
	}
	return true
}

func configCacheImportValid(fs afero.Fs, item configCacheImport) bool {
	if item.Importer == "" || item.Path == "" {
		return false
	}
	matches, err := resolveImportPaths(fs, item.Importer, item.Path)
	if err != nil {
		return false
	}
	return slices.Equal(matches, item.Matches)
}

func (l *Loader) configCacheImportsValid(imports []configCacheImport) bool {
	for _, item := range imports {
		if !configCacheImportValid(l.fs, item) {
			return false
		}
	}
	return true
}

func configCachePackageValid(fs afero.Fs, workDir string, item configCachePackage) bool {
	if item.Pattern == "" {
		return false
	}
	matches, err := resolvePackageBuildFiles(fs, workDir, item.Pattern)
	if err != nil {
		return false
	}
	return slices.Equal(matches, item.Matches)
}

func (l *Loader) configCachePackagesValid(packages []configCachePackage) bool {
	for _, item := range packages {
		if !configCachePackageValid(l.fs, l.cfg.WorkDir, item) {
			return false
		}
	}
	return true
}

func configCacheEnvValid(item configCacheEnv) bool {
	return item.Name != "" && os.Getenv(item.Name) == item.Value
}

func configCacheEnvsValid(envs []configCacheEnv) bool {
	for _, item := range envs {
		if !configCacheEnvValid(item) {
			return false
		}
	}
	return true
}
