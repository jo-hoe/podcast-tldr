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

// Run backs up every not-yet-backed-up episode that has a bundle. Each episode
// is committed and pushed individually so the archive is updated incrementally —
// a crash or interruption leaves already-processed episodes safely in the remote.
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

	backed := 0
	for pi := range m.Podcasts {
		podcast := &m.Podcasts[pi]
		for ei := range podcast.Episodes {
			ep := &podcast.Episodes[ei]
			if !s.eligible(ep) {
				continue
			}
			if err := s.backupEpisode(repo, podcast.ShowTitle, ep, m); err != nil {
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

// backupEpisode stages, commits, pushes, marks backed-up, and saves the manifest
// for a single episode. Any error aborts the episode but leaves the repo clean.
func (s *Service) backupEpisode(repo gitrepo.Repository, showTitle string, ep *manifest.Episode, m *manifest.Manifest) error {
	if err := s.stageEpisode(repo, showTitle, ep); err != nil {
		return err
	}

	msg := fmt.Sprintf("Back up episode: %s", ep.ID)
	if err := s.commitAndPush(repo, msg); err != nil {
		return err
	}

	ep.BackedUp = true
	if err := m.Save(s.cfg.WorkDir); err != nil {
		// Non-fatal: the episode is in the archive; worst case it gets backed up
		// again on the next run (idempotent — same content, duplicate commit).
		slog.Warn("backed up but failed to save manifest", "id", ep.ID, "err", err)
	}
	return nil
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

// workPath resolves a work-relative manifest path to an absolute local path.
func (s *Service) workPath(rel string) string {
	return filepath.Join(s.cfg.WorkDir, filepath.FromSlash(rel))
}
