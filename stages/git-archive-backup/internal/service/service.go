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
	cfg    *config.Config
	opener Opener
}

// New constructs a Service.
func New(cfg *config.Config, opener Opener) *Service {
	return &Service{cfg: cfg, opener: opener}
}

// Run backs up every not-yet-backed-up episode that has a bundle. It returns an
// error for failures that abort the whole stage (loading the manifest, opening or
// pushing the repo, saving the manifest). Per-episode file problems are logged.
func (s *Service) Run() error {
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

	staged := s.stageEpisodes(m, repo)
	if staged == 0 {
		slog.Info("no new episodes to back up")
		return nil
	}

	if err := s.commitAndPush(repo, staged); err != nil {
		return err
	}

	if err := m.Save(s.cfg.WorkDir); err != nil {
		return fmt.Errorf("failed to save manifest: %w", err)
	}
	slog.Info("backup stage complete", "episodes", staged)
	return nil
}

// stageEpisodes copies each eligible episode's artifacts into the repo and marks
// it backed up in the manifest. It returns the number of episodes staged.
func (s *Service) stageEpisodes(m *manifest.Manifest, repo gitrepo.Repository) int {
	staged := 0
	for pi := range m.Podcasts {
		podcast := &m.Podcasts[pi]
		for ei := range podcast.Episodes {
			ep := &podcast.Episodes[ei]
			if !s.eligible(ep) {
				continue
			}
			if err := s.stageEpisode(repo, podcast.ShowTitle, ep); err != nil {
				slog.Error("skipping episode", "id", ep.ID, "err", err)
				continue
			}
			ep.BackedUp = true
			staged++
		}
	}
	return staged
}

// eligible reports whether an episode should be backed up: it must have a bundle
// and not already be backed up.
func (s *Service) eligible(ep *manifest.Episode) bool {
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

func (s *Service) commitAndPush(repo gitrepo.Repository, staged int) error {
	msg := fmt.Sprintf("Back up %d podcast episode(s)", staged)
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

// workPath resolves a work-relative manifest path to an absolute local path.
func (s *Service) workPath(rel string) string {
	return filepath.Join(s.cfg.WorkDir, filepath.FromSlash(rel))
}
