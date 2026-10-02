package gitrepo

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// initBareRemote creates a bare repository with an initial commit on the given
// branch, so Open can clone a single existing branch from it. It returns the
// path to the bare repo (usable as a file:// remote URL / local path).
func initBareRemote(t *testing.T, branch string) string {
	t.Helper()
	base := t.TempDir()
	barePath := filepath.Join(base, "remote.git")

	// Seed via a normal repo, then push into the bare remote so the branch exists.
	seedPath := filepath.Join(base, "seed")
	seed, err := git.PlainInit(seedPath, false)
	if err != nil {
		t.Fatalf("init seed: %v", err)
	}
	wt, err := seed.Worktree()
	if err != nil {
		t.Fatalf("seed worktree: %v", err)
	}
	readme := filepath.Join(seedPath, "README.md")
	if err := os.WriteFile(readme, []byte("archive\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("stage seed file: %v", err)
	}
	if _, err := wt.Commit("init", &git.CommitOptions{
		Author: signature(),
	}); err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	// Move HEAD to the desired branch name.
	branchRef := plumbing.NewBranchReferenceName(branch)
	if err := wt.Checkout(&git.CheckoutOptions{Branch: branchRef, Create: true}); err != nil {
		t.Fatalf("seed checkout branch: %v", err)
	}

	if _, err := git.PlainInit(barePath, true); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	if _, err := seed.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{barePath}}); err != nil {
		t.Fatalf("create remote: %v", err)
	}
	if err := seed.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("seed push: %v", err)
	}
	return barePath
}

func signature() *object.Signature {
	return &object.Signature{Name: "Seed", Email: "seed@example.com", When: time.Unix(0, 0)}
}

func TestGoGitRepository_RoundTrip(t *testing.T) {
	// Pin commit time so the test is deterministic.
	now = func() time.Time { return time.Unix(1700000000, 0) }
	t.Cleanup(func() { now = time.Now })

	remote := initBareRemote(t, "main")

	repo, err := Open(Options{
		RepoURL:     remote,
		Branch:      "main",
		CheckoutDir: filepath.Join(t.TempDir(), "checkout"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := repo.AddFile("podcasts/my-show/ep-001/summary.md", []byte("# Summary")); err != nil {
		t.Fatalf("AddFile summary: %v", err)
	}
	if err := repo.AddFile("podcasts/my-show/ep-001/bundle.zip", []byte("ZIPDATA")); err != nil {
		t.Fatalf("AddFile bundle: %v", err)
	}
	if err := repo.Commit("back up ep-001", Author{Name: "Tester", Email: "tester@example.com"}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := repo.Push(); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify by re-cloning the bare remote into a fresh directory.
	verifyDir := filepath.Join(t.TempDir(), "verify")
	if _, err := git.PlainClone(verifyDir, false, &git.CloneOptions{
		URL:           remote,
		ReferenceName: plumbing.NewBranchReferenceName("main"),
		SingleBranch:  true,
	}); err != nil {
		t.Fatalf("verify clone: %v", err)
	}

	assertFile(t, filepath.Join(verifyDir, "podcasts", "my-show", "ep-001", "summary.md"), "# Summary")
	assertFile(t, filepath.Join(verifyDir, "podcasts", "my-show", "ep-001", "bundle.zip"), "ZIPDATA")
}

func TestGoGitRepository_CommitNothingReturnsSentinel(t *testing.T) {
	remote := initBareRemote(t, "main")
	repo, err := Open(Options{
		RepoURL:     remote,
		Branch:      "main",
		CheckoutDir: filepath.Join(t.TempDir(), "checkout"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := repo.Commit("noop", Author{Name: "T", Email: "t@e.com"}); err != ErrNothingToCommit {
		t.Fatalf("expected ErrNothingToCommit, got %v", err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, string(got), want)
	}
}
