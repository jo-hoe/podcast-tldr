package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jo-hoe/git-archive-backup/internal/config"
	"github.com/jo-hoe/git-archive-backup/internal/gitrepo"
	manifest "github.com/jo-hoe/manifest-lib"
)

// fakeRepository records everything staged, committed, and pushed so tests can
// assert the service drives the git layer correctly without touching real git.
type fakeRepository struct {
	files     map[string][]byte
	commits   []string
	author    gitrepo.Author
	pushed    int
	commitErr error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{files: map[string][]byte{}}
}

func (f *fakeRepository) AddFile(relPath string, content []byte) error {
	f.files[relPath] = content
	return nil
}

func (f *fakeRepository) Commit(message string, author gitrepo.Author) error {
	if f.commitErr != nil {
		return f.commitErr
	}
	f.commits = append(f.commits, message)
	f.author = author
	return nil
}

func (f *fakeRepository) Push() error {
	f.pushed++
	return nil
}

// setup writes a work directory with a manifest and the referenced artifacts,
// and returns a config pointing at it plus the fake repository.
func setup(t *testing.T, m *manifest.Manifest, artifacts map[string]string) (*config.Config, *fakeRepository) {
	t.Helper()
	workDir := t.TempDir()
	for rel, body := range artifacts {
		abs := filepath.Join(workDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatalf("write artifact: %v", err)
		}
	}
	if err := m.Save(workDir); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	cfg := &config.Config{
		WorkDir:     workDir,
		RepoURL:     "https://example.com/archive.git",
		Branch:      "main",
		AuthorName:  "Tester",
		AuthorEmail: "tester@example.com",
		CheckoutDir: filepath.Join(workDir, ".checkout"),
	}
	return cfg, newFakeRepository()
}

func manifestWithEpisode(bundle, summary string) *manifest.Manifest {
	return &manifest.Manifest{
		Podcasts: []manifest.Podcast{
			{
				ShowTitle: "My Show",
				Episodes: []manifest.Episode{
					{ID: "ep-001", Title: "First", BundleFile: bundle, SummaryFile: summary},
				},
			},
		},
	}
}

func TestRun_StagesBundleAndSummary(t *testing.T) {
	m := manifestWithEpisode("bundles/ep-001.zip", "summaries/ep-001.md")
	cfg, fake := setup(t, m, map[string]string{
		"bundles/ep-001.zip":  "ZIPDATA",
		"summaries/ep-001.md": "# Summary",
	})

	svc := New(cfg, func(gitrepo.Options) (gitrepo.Repository, error) { return fake, nil })
	if err := svc.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if got := string(fake.files["podcasts/my-show/ep-001/bundle.zip"]); got != "ZIPDATA" {
		t.Errorf("bundle not staged at expected path, got %q", got)
	}
	if got := string(fake.files["podcasts/my-show/ep-001/summary.md"]); got != "# Summary" {
		t.Errorf("summary not staged at expected path, got %q", got)
	}
	if len(fake.commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(fake.commits))
	}
	if fake.author.Name != "Tester" || fake.author.Email != "tester@example.com" {
		t.Errorf("unexpected commit author: %+v", fake.author)
	}
	if fake.pushed != 1 {
		t.Errorf("expected 1 push, got %d", fake.pushed)
	}
}

func TestRun_SetsBackedUpAndSavesManifest(t *testing.T) {
	m := manifestWithEpisode("bundles/ep-001.zip", "summaries/ep-001.md")
	cfg, fake := setup(t, m, map[string]string{
		"bundles/ep-001.zip":  "ZIPDATA",
		"summaries/ep-001.md": "# Summary",
	})

	svc := New(cfg, func(gitrepo.Options) (gitrepo.Repository, error) { return fake, nil })
	if err := svc.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	reloaded, err := manifest.Load(cfg.WorkDir)
	if err != nil {
		t.Fatalf("reload manifest: %v", err)
	}
	if !reloaded.Podcasts[0].Episodes[0].BackedUp {
		t.Error("expected episode BackedUp=true after backup")
	}
}

func TestRun_SkipsEpisodesWithoutBundle(t *testing.T) {
	m := manifestWithEpisode("", "")
	cfg, fake := setup(t, m, nil)

	svc := New(cfg, func(gitrepo.Options) (gitrepo.Repository, error) { return fake, nil })
	if err := svc.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(fake.files) != 0 {
		t.Errorf("expected nothing staged, got %d files", len(fake.files))
	}
	if len(fake.commits) != 0 {
		t.Errorf("expected no commit, got %d", len(fake.commits))
	}
}

func TestRun_SkipsAlreadyBackedUp(t *testing.T) {
	m := manifestWithEpisode("bundles/ep-001.zip", "summaries/ep-001.md")
	m.Podcasts[0].Episodes[0].BackedUp = true
	cfg, fake := setup(t, m, map[string]string{
		"bundles/ep-001.zip":  "ZIPDATA",
		"summaries/ep-001.md": "# Summary",
	})

	svc := New(cfg, func(gitrepo.Options) (gitrepo.Repository, error) { return fake, nil })
	if err := svc.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(fake.files) != 0 {
		t.Errorf("expected nothing staged for already-backed-up episode, got %d", len(fake.files))
	}
}

func TestRun_BundleWithoutSummary(t *testing.T) {
	m := manifestWithEpisode("bundles/ep-001.zip", "")
	cfg, fake := setup(t, m, map[string]string{
		"bundles/ep-001.zip": "ZIPDATA",
	})

	svc := New(cfg, func(gitrepo.Options) (gitrepo.Repository, error) { return fake, nil })
	if err := svc.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if _, ok := fake.files["podcasts/my-show/ep-001/bundle.zip"]; !ok {
		t.Error("expected bundle staged")
	}
	if _, ok := fake.files["podcasts/my-show/ep-001/summary.md"]; ok {
		t.Error("did not expect a summary to be staged")
	}
}

func TestRun_ToleratesNothingToCommit(t *testing.T) {
	m := manifestWithEpisode("bundles/ep-001.zip", "")
	cfg, fake := setup(t, m, map[string]string{
		"bundles/ep-001.zip": "ZIPDATA",
	})
	fake.commitErr = gitrepo.ErrNothingToCommit

	svc := New(cfg, func(gitrepo.Options) (gitrepo.Repository, error) { return fake, nil })
	if err := svc.Run(); err != nil {
		t.Fatalf("Run should tolerate ErrNothingToCommit, got: %v", err)
	}
	if fake.pushed != 0 {
		t.Errorf("expected no push when nothing to commit, got %d", fake.pushed)
	}
}

func TestRun_ReturnsErrorWhenOpenerFails(t *testing.T) {
	m := manifestWithEpisode("bundles/ep-001.zip", "")
	cfg, _ := setup(t, m, map[string]string{"bundles/ep-001.zip": "ZIPDATA"})

	sentinel := errors.New("boom")
	svc := New(cfg, func(gitrepo.Options) (gitrepo.Repository, error) { return nil, sentinel })
	if err := svc.Run(); !errors.Is(err, sentinel) {
		t.Fatalf("expected opener error to propagate, got %v", err)
	}
}
