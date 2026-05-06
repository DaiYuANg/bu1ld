package git

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/samber/oops"
	"github.com/spf13/afero"
)

type InfoHandler struct{}

func NewInfoHandler() *InfoHandler {
	return &InfoHandler{}
}

func (h *InfoHandler) Kind() string {
	return InfoActionKind
}

func (h *InfoHandler) Run(_ context.Context, workDir string, params map[string]any, _ io.Writer) error {
	spec := infoSpecFromParams(params)
	info, err := readInfo(workDir, spec)
	if err != nil {
		return err
	}
	return writeInfo(workDir, spec.Out, info)
}

type infoSpec struct {
	Repo         string
	Out          string
	IncludeDirty bool
}

type Info struct {
	Head         string    `json:"head"`
	Branch       string    `json:"branch,omitempty"`
	Commit       Commit    `json:"commit"`
	Dirty        bool      `json:"dirty,omitempty"`
	Remotes      []Remote  `json:"remotes,omitempty"`
	GeneratedAt  time.Time `json:"generatedAt"`
	Repository   string    `json:"repository"`
	DetachedHead bool      `json:"detachedHead,omitempty"`
}

type Commit struct {
	Hash      string    `json:"hash"`
	ShortHash string    `json:"shortHash"`
	Author    string    `json:"author,omitempty"`
	Email     string    `json:"email,omitempty"`
	Message   string    `json:"message,omitempty"`
	Time      time.Time `json:"time"`
}

type Remote struct {
	Name string   `json:"name"`
	URLs []string `json:"urls"`
}

func infoSpecFromParams(params map[string]any) infoSpec {
	repo := stringParam(params, "repo")
	if repo == "" {
		repo = "."
	}
	return infoSpec{
		Repo:         repo,
		Out:          stringParam(params, "out"),
		IncludeDirty: boolParam(params, "include_dirty"),
	}
}

func readInfo(workDir string, spec infoSpec) (Info, error) {
	repoPath := absolutePath(workDir, spec.Repo)
	repository, err := gogit.PlainOpenWithOptions(repoPath, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return Info{}, oops.In("bu1ld.git").
			With("repo", spec.Repo).
			Wrapf(err, "open git repository")
	}
	head, err := repository.Head()
	if err != nil {
		return Info{}, oops.In("bu1ld.git").
			With("repo", spec.Repo).
			Wrapf(err, "read git head")
	}
	commit, err := repository.CommitObject(head.Hash())
	if err != nil {
		return Info{}, oops.In("bu1ld.git").
			With("repo", spec.Repo).
			With("head", head.Hash().String()).
			Wrapf(err, "read git commit")
	}
	info := Info{
		Head:         head.Hash().String(),
		Branch:       branchName(head),
		Commit:       commitInfo(commit),
		Remotes:      remotes(repository),
		GeneratedAt:  time.Now().UTC(),
		Repository:   filepath.ToSlash(spec.Repo),
		DetachedHead: !head.Name().IsBranch(),
	}
	if spec.IncludeDirty {
		dirty, err := dirtyWorktree(repository, spec.Repo)
		if err != nil {
			return Info{}, err
		}
		info.Dirty = dirty
	}
	return info, nil
}

func branchName(head *plumbing.Reference) string {
	if !head.Name().IsBranch() {
		return ""
	}
	return head.Name().Short()
}

func commitInfo(commit *object.Commit) Commit {
	hash := commit.Hash.String()
	return Commit{
		Hash:      hash,
		ShortHash: shortHash(hash),
		Author:    commit.Author.Name,
		Email:     commit.Author.Email,
		Message:   commit.Message,
		Time:      commit.Author.When,
	}
}

func dirtyWorktree(repository *gogit.Repository, repo string) (bool, error) {
	worktree, err := repository.Worktree()
	if err != nil {
		return false, oops.In("bu1ld.git").
			With("repo", repo).
			Wrapf(err, "open git worktree")
	}
	status, err := worktree.Status()
	if err != nil {
		return false, oops.In("bu1ld.git").
			With("repo", repo).
			Wrapf(err, "read git worktree status")
	}
	return !status.IsClean(), nil
}

func remotes(repository *gogit.Repository) []Remote {
	items, err := repository.Remotes()
	if err != nil {
		return nil
	}
	result := make([]Remote, 0, len(items))
	for _, item := range items {
		result = append(result, Remote{Name: item.Config().Name, URLs: item.Config().URLs})
	}
	return result
}

func writeInfo(workDir, out string, info Info) error {
	path := filepath.Join(workDir, out)
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return oops.In("bu1ld.git").Wrapf(err, "marshal git info")
	}
	fs := afero.NewOsFs()
	if err := fs.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return oops.In("bu1ld.git").With("out", out).Wrapf(err, "create git info output directory")
	}
	if err := afero.WriteFile(fs, path, append(data, '\n'), 0o600); err != nil {
		return oops.In("bu1ld.git").With("out", out).Wrapf(err, "write git info")
	}
	return nil
}

func stringParam(params map[string]any, key string) string {
	value, ok := params[key].(string)
	if !ok {
		return ""
	}
	return value
}

func boolParam(params map[string]any, key string) bool {
	value, ok := params[key].(bool)
	if !ok {
		return false
	}
	return value
}

func absolutePath(workDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workDir, path)
}

func shortHash(hash string) string {
	const shortLength = 12
	if len(hash) <= shortLength {
		return hash
	}
	return hash[:shortLength]
}
