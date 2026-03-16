package main

import (
	"fmt"
	"path/filepath"

	"github.com/jallum/beadwork/internal/issue"
	"github.com/jallum/beadwork/internal/repo"
)

// RepoSource wraps a beadwork repository and its issue store for TUI use.
type RepoSource struct {
	Name   string       // short display name (e.g. "toolbox")
	Path   string       // absolute filesystem path
	Repo   *repo.Repo
	Store  *issue.Store
	Prefix string       // issue prefix (e.g. "tb")
}

// OpenRepo opens a beadwork repository at the given path and returns a
// read-only RepoSource. The name defaults to the directory basename.
func OpenRepo(path string) (*RepoSource, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path %s: %w", path, err)
	}

	r, err := repo.FindRepoAt(abs)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", abs, err)
	}
	if !r.IsInitialized() {
		return nil, fmt.Errorf("%s: beadwork not initialized", abs)
	}

	store := issue.NewStore(r.TreeFS(), r.Prefix)
	// Read-only: no Committer set

	return &RepoSource{
		Name:   filepath.Base(abs),
		Path:   abs,
		Repo:   r,
		Store:  store,
		Prefix: r.Prefix,
	}, nil
}
