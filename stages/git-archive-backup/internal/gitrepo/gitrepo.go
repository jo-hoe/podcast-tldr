// Package gitrepo wraps the git operations the backup stage needs behind a small
// Repository interface. This keeps all go-git usage in one place and lets the
// service be tested against an in-memory fake.
package gitrepo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Author identifies the committer used for archive commits.
type Author struct {
	Name  string
	Email string
}

// Repository is the minimal set of git operations the backup service needs. All
// paths passed to AddFile are repo-relative with forward slashes.
type Repository interface {
	// AddFile writes content to the repo-relative path (creating directories as
	// needed) and stages it.
	AddFile(relPath string, content []byte) error
	// Commit records all staged changes with the given message. It returns
	// ErrNothingToCommit when the working tree has no changes.
	Commit(message string, author Author) error
	// Push publishes committed changes to the configured remote branch.
	Push() error
}

// ErrNothingToCommit is returned by Commit when there is nothing new to record.
var ErrNothingToCommit = errors.New("nothing to commit")

// GoGitRepository is the production Repository backed by go-git working on a
// local checkout of the remote archive repository.
type GoGitRepository struct {
	repo     *git.Repository
	worktree *git.Worktree
	dir      string
	branch   string
	auth     transport.AuthMethod
}

// Options configures how the archive repository is opened.
type Options struct {
	// RepoURL is the HTTPS URL of the remote archive repository.
	RepoURL string
	// Token is the access token used for HTTP basic auth. May be empty for
	// public/unauthenticated remotes (e.g. a local bare repo in tests).
	Token string
	// Branch is the branch to checkout, commit to, and push.
	Branch string
	// CheckoutDir is the local directory the repository is cloned into.
	CheckoutDir string
}

// Open clones the archive repository into the checkout directory, or opens and
// fast-forwards it if the checkout already exists. It checks out the target
// branch, creating it from the current HEAD when the remote has no such branch.
func Open(opts Options) (*GoGitRepository, error) {
	auth := authMethod(opts.Token)
	branchRef := plumbing.NewBranchReferenceName(opts.Branch)

	repo, err := openOrClone(opts, auth, branchRef)
	if err != nil {
		return nil, err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to open worktree: %w", err)
	}

	if err := checkoutBranch(worktree, branchRef); err != nil {
		return nil, err
	}

	return &GoGitRepository{
		repo:     repo,
		worktree: worktree,
		dir:      opts.CheckoutDir,
		branch:   opts.Branch,
		auth:     auth,
	}, nil
}

func authMethod(token string) transport.AuthMethod {
	if token == "" {
		return nil
	}
	// For token auth the username can be any non-empty string.
	return &http.BasicAuth{Username: "git", Password: token}
}

func openOrClone(opts Options, auth transport.AuthMethod, branchRef plumbing.ReferenceName) (*git.Repository, error) {
	if _, err := os.Stat(filepath.Join(opts.CheckoutDir, ".git")); err == nil {
		repo, err := git.PlainOpen(opts.CheckoutDir)
		if err != nil {
			return nil, fmt.Errorf("failed to open existing checkout: %w", err)
		}
		return repo, nil
	}

	if err := os.MkdirAll(opts.CheckoutDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create checkout dir: %w", err)
	}

	repo, err := git.PlainClone(opts.CheckoutDir, false, &git.CloneOptions{
		URL:           opts.RepoURL,
		Auth:          auth,
		ReferenceName: branchRef,
		SingleBranch:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to clone %s: %w", opts.RepoURL, err)
	}
	return repo, nil
}

// checkoutBranch checks out branchRef, creating it at the current HEAD if it does
// not exist yet (e.g. the branch was cloned under a different default name).
func checkoutBranch(worktree *git.Worktree, branchRef plumbing.ReferenceName) error {
	err := worktree.Checkout(&git.CheckoutOptions{Branch: branchRef})
	if err == nil {
		return nil
	}
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return fmt.Errorf("failed to checkout branch: %w", err)
	}
	if createErr := worktree.Checkout(&git.CheckoutOptions{Branch: branchRef, Create: true}); createErr != nil {
		return fmt.Errorf("failed to create branch: %w", createErr)
	}
	return nil
}

// AddFile writes content to a repo-relative path and stages it.
func (r *GoGitRepository) AddFile(relPath string, content []byte) error {
	abs := filepath.Join(r.dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("failed to create dir for %s: %w", relPath, err)
	}
	if err := os.WriteFile(abs, content, 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", relPath, err)
	}
	if _, err := r.worktree.Add(relPath); err != nil {
		return fmt.Errorf("failed to stage %s: %w", relPath, err)
	}
	return nil
}

// Commit records all staged changes. It returns ErrNothingToCommit when the tree
// is clean.
func (r *GoGitRepository) Commit(message string, author Author) error {
	status, err := r.worktree.Status()
	if err != nil {
		return fmt.Errorf("failed to read status: %w", err)
	}
	if status.IsClean() {
		return ErrNothingToCommit
	}

	_, err = r.worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{Name: author.Name, Email: author.Email, When: now()},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}

// Push publishes the current branch to the origin remote.
func (r *GoGitRepository) Push() error {
	refspec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", r.branch, r.branch))
	err := r.repo.Push(&git.PushOptions{
		Auth:     r.auth,
		RefSpecs: []config.RefSpec{refspec},
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("failed to push: %w", err)
	}
	return nil
}
