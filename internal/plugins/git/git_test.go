package git_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	gitplugin "github.com/lyonbrown4d/bu1ld/internal/plugins/git"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
)

func TestInfoHandlerWritesGitMetadata(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	commitHash := initRepository(t, workDir)

	handler := gitplugin.NewInfoHandler()
	err := handler.Run(context.Background(), workDir, map[string]any{
		"repo":          ".",
		"out":           "dist/git-info.json",
		"include_dirty": true,
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	data, err := afero.ReadFile(afero.NewOsFs(), filepath.Join(workDir, "dist", "git-info.json"))
	if err != nil {
		t.Fatalf("read git info: %v", err)
	}
	var info gitplugin.Info
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("decode git info: %v", err)
	}
	if info.Head != commitHash {
		t.Fatalf("head = %q, want %q", info.Head, commitHash)
	}
	if info.Branch != "master" {
		t.Fatalf("branch = %q, want master", info.Branch)
	}
	if info.Dirty {
		t.Fatalf("dirty = true, want false")
	}
}

func initRepository(t *testing.T, workDir string) string {
	t.Helper()

	repository, err := gogit.PlainInit(workDir, false)
	if err != nil {
		t.Fatalf("init repository: %v", err)
	}
	if writeErr := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# test\n"), 0o600); writeErr != nil {
		t.Fatalf("write readme: %v", writeErr)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatalf("open worktree: %v", err)
	}
	if _, addErr := worktree.Add("README.md"); addErr != nil {
		t.Fatalf("add readme: %v", addErr)
	}
	hash, err := worktree.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Unix(1_700_000_000, 0),
		},
	})
	if err != nil {
		t.Fatalf("commit readme: %v", err)
	}
	return hash.String()
}
