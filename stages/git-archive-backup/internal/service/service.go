// Package service orchestrates the backup stage: it reads the shared manifest,
// copies each episode's bundle and summary into the archive repository at an
// intuitive layout, commits and pushes, then marks the episodes backed up and
// writes the manifest back.
package service

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jo-hoe/git-archive-backup/internal/config"
	"github.com/jo-hoe/git-archive-backup/internal/gitrepo"
	"github.com/jo-hoe/git-archive-backup/internal/layout"
	manifest "github.com/jo-hoe/manifest-lib"
)

// Opener constructs a Repository for the given options. It is injected so the
// service can be tested with a fake repository.
type Opener func(gitrepo.Options) (gitrepo.Repository, error)

// Service runs the backup stage.
type Service struct {
	cfg       *config.Config
	opener    Opener
	episodeID string // when non-empty, only this episode is processed
}

// New constructs a Service.
// episodeID may be empty to process all eligible episodes.
func New(cfg *config.Config, opener Opener, episodeID string) *Service {
	return &Service{cfg: cfg, opener: opener, episodeID: episodeID}
}

// Run backs up every not-yet-backed-up episode that has a bundle. Each episode
// is committed and pushed individually so the archive is updated incrementally.
//
// A file lock serialises the entire git operation (open + stage + commit + push)
// so parallel fan-out containers sharing the work directory never corrupt the
// shared archive worktree.
func (s *Service) Run() error {
	lockPath := filepath.Join(s.cfg.WorkDir, ".git-op.lock")
	if err := acquireGitLock(lockPath, 15*time.Minute); err != nil {
		return fmt.Errorf("failed to acquire git lock: %w", err)
	}
	defer releaseGitLock(lockPath)

	m, err := manifest.Load(s.cfg.WorkDir)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	repo, err := s.opener(gitrepo.Options{
		RepoURL:     s.cfg.RepoURL,
		Token:       s.cfg.Token(),
		Branch:      s.cfg.Branch,
		CheckoutDir: s.cfg.CheckoutDir,
	})
	if err != nil {
		return fmt.Errorf("failed to open archive repository: %w", err)
	}

	backed := 0
	for pi := range m.Podcasts {
		podcast := &m.Podcasts[pi]
		for ei := range podcast.Episodes {
			ep := &podcast.Episodes[ei]
			if !s.eligible(ep) {
				continue
			}
			if err := s.stageAndCommitEpisode(repo, podcast.ShowTitle, ep, m); err != nil {
				slog.Error("skipping episode", "id", ep.ID, "err", err)
				continue
			}
			backed++
		}
	}

	if backed == 0 {
		slog.Info("no new episodes to back up")
	} else {
		slog.Info("backup stage complete", "episodes", backed)
	}
	return nil
}

// stageAndCommitEpisode stages, commits, pushes, and marks one episode backed-up.
// The caller (Run) holds the git-op lock for the duration.
func (s *Service) stageAndCommitEpisode(repo gitrepo.Repository, showTitle string, ep *manifest.Episode, m *manifest.Manifest) error {
	if err := s.stageEpisode(repo, showTitle, ep); err != nil {
		return err
	}
	msg := fmt.Sprintf("Back up episode: %s", ep.ID)
	if err := s.commitAndPush(repo, msg); err != nil {
		return err
	}
	ep.BackedUp = true
	if err := m.Save(s.cfg.WorkDir); err != nil {
		// Non-fatal: episode is in the archive; worst case it gets backed up again.
		slog.Warn("backed up but failed to save manifest", "id", ep.ID, "err", err)
	}
	return nil
}

// eligible reports whether an episode should be backed up: it must have a bundle,
// not already be backed up, and (when episodeID is set) match the target ID.
func (s *Service) eligible(ep *manifest.Episode) bool {
	if s.episodeID != "" && ep.ID != s.episodeID {
		return false
	}
	return ep.BundleFile != "" && !ep.BackedUp
}

// stageEpisode copies one episode's bundle and (optional) summary into the repo.
func (s *Service) stageEpisode(repo gitrepo.Repository, showTitle string, ep *manifest.Episode) error {
	bundle, err := os.ReadFile(s.workPath(ep.BundleFile))
	if err != nil {
		return fmt.Errorf("read bundle: %w", err)
	}
	if err := repo.AddFile(layout.BundlePath(showTitle, ep.ID), bundle); err != nil {
		return err
	}
	if ep.SummaryFile != "" {
		summary, err := os.ReadFile(s.workPath(ep.SummaryFile))
		if err != nil {
			return fmt.Errorf("read summary: %w", err)
		}
		if err := repo.AddFile(layout.SummaryPath(showTitle, ep.ID), summary); err != nil {
			return err
		}
	}
	slog.Info("staged episode", "id", ep.ID, "dir", layout.EpisodeDir(showTitle, ep.ID))
	return nil
}

func (s *Service) commitAndPush(repo gitrepo.Repository, msg string) error {
	err := repo.Commit(msg, gitrepo.Author{Name: s.cfg.AuthorName, Email: s.cfg.AuthorEmail})
	if errors.Is(err, gitrepo.ErrNothingToCommit) {
		slog.Info("archive already up to date, nothing to commit")
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to commit archive: %w", err)
	}
	if err := repo.Push(); err != nil {
		return fmt.Errorf("failed to push archive: %w", err)
	}
	return nil
}

func acquireGitLock(lockPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	sleep := 200 * time.Millisecond
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return nil
		}
		if !os.IsExist(err) {
			return fmt.Errorf("lock error: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for git lock after %s", timeout)
		}
		time.Sleep(sleep)
		if sleep < 5*time.Second {
			sleep = sleep * 3 / 2
		}
	}
}

func releaseGitLock(lockPath string) { _ = os.Remove(lockPath) }

// workPath resolves a work-relative manifest path to an absolute local path.
func (s *Service) workPath(rel string) string {
	return filepath.Join(s.cfg.WorkDir, filepath.FromSlash(rel))
}
