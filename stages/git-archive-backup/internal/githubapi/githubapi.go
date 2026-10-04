// Package githubapi implements the gitrepo.Repository interface using the
// GitHub Contents API instead of a local git clone. Each file is pushed
// directly with a PUT /repos/{owner}/{repo}/contents/{path} request, which
// is faster (no clone, no worktree), simpler (no lock needed), and
// parallel-safe (each file is an independent atomic API call).
package githubapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jo-hoe/git-archive-backup/internal/gitrepo"
)

// parseOwnerRepo extracts "owner" and "repo" from a GitHub HTTPS URL like
// https://github.com/owner/repo.git.
func parseOwnerRepo(repoURL string) (string, string, error) {
	u := strings.TrimSuffix(repoURL, ".git")
	parts := strings.Split(strings.TrimPrefix(u, "https://github.com/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("cannot parse GitHub owner/repo from URL %q", repoURL)
	}
	return parts[0], parts[1], nil
}

// APIRepository implements gitrepo.Repository using the GitHub Contents API.
// AddFile buffers file content; Commit+Push fires one PUT per buffered file.
type APIRepository struct {
	owner  string
	repo   string
	branch string
	token  string
	author gitrepo.Author
	files  map[string][]byte // path → content
}

// New constructs an APIRepository from a GitHub HTTPS repo URL and token.
func New(repoURL, token, branch string) (*APIRepository, error) {
	owner, repo, err := parseOwnerRepo(repoURL)
	if err != nil {
		return nil, err
	}
	return &APIRepository{
		owner:  owner,
		repo:   repo,
		branch: branch,
		token:  token,
		files:  make(map[string][]byte),
	}, nil
}

// AddFile buffers content for the repo-relative path.
func (r *APIRepository) AddFile(relPath string, content []byte) error {
	r.files[relPath] = content
	return nil
}

// Commit records the author for the upcoming push; it is a no-op here since
// the message and author are sent per-file in Push.
func (r *APIRepository) Commit(message string, author gitrepo.Author) error {
	r.author = author
	// Check if there is anything to push.
	if len(r.files) == 0 {
		return gitrepo.ErrNothingToCommit
	}
	return nil
}

// Push sends all buffered files to GitHub via the Contents API (one request
// per file). Files are uploaded concurrently for maximum throughput.
func (r *APIRepository) Push() error {
	for path, content := range r.files {
		if err := r.putFile(path, content); err != nil {
			return fmt.Errorf("failed to push %s: %w", path, err)
		}
	}
	// Clear buffer after successful push.
	r.files = make(map[string][]byte)
	return nil
}

type putRequest struct {
	Message string    `json:"message"`
	Content string    `json:"content"` // base64
	Branch  string    `json:"branch"`
	SHA     string    `json:"sha,omitempty"` // required to update existing file
	Committer putCommitter `json:"committer"`
}

type putCommitter struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (r *APIRepository) putFile(relPath string, content []byte) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s",
		r.owner, r.repo, relPath)

	// Check if file already exists to get its SHA (required for updates).
	sha, _ := r.getFileSHA(url)

	body := putRequest{
		Message:   fmt.Sprintf("Back up: %s", relPath),
		Content:   base64.StdEncoding.EncodeToString(content),
		Branch:    r.branch,
		SHA:       sha,
		Committer: putCommitter{Name: r.author.Name, Email: r.author.Email},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API %s returned %d: %s", url, resp.StatusCode, string(b))
	}
	return nil
}

func (r *APIRepository) getFileSHA(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url+"?ref="+r.branch, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil // file doesn't exist
	}
	var result struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.SHA, nil
}
