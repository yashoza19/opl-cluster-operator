package git

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// Client handles Git operations for cluster configuration management
type Client struct {
	repoURL     string
	branch      string
	localPath   string
	auth        transport.AuthMethod
	authorName  string
	authorEmail string
}

// Config holds configuration for the Git client
type Config struct {
	RepoURL     string
	Branch      string
	LocalPath   string
	SSHKeyPath  string // Path to SSH private key file
	Username    string // For HTTPS auth
	Password    string // For HTTPS auth (or personal access token)
	AuthorName  string
	AuthorEmail string
}

// NewClient creates a new Git client
func NewClient(cfg Config) (*Client, error) {
	client := &Client{
		repoURL:     cfg.RepoURL,
		branch:      cfg.Branch,
		localPath:   cfg.LocalPath,
		authorName:  cfg.AuthorName,
		authorEmail: cfg.AuthorEmail,
	}

	// Set up authentication
	if cfg.SSHKeyPath != "" {
		// Read SSH key file
		keyBytes, err := os.ReadFile(cfg.SSHKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read SSH key: %w", err)
		}

		// Parse private key
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SSH key: %w", err)
		}

		// Create SSH auth with insecure host key callback
		// Disable host key checking (insecure but simplifies setup for demo)
		publicKeys := &gitssh.PublicKeys{
			User:   "git",
			Signer: signer,
		}
		publicKeys.HostKeyCallback = ssh.InsecureIgnoreHostKey()
		client.auth = publicKeys
	} else if cfg.Username != "" && cfg.Password != "" {
		client.auth = &http.BasicAuth{
			Username: cfg.Username,
			Password: cfg.Password,
		}
	}

	return client, nil
}

// Initialize clones the repository or opens existing clone
func (c *Client) Initialize() error {
	// Check if repository already exists
	if _, err := os.Stat(filepath.Join(c.localPath, ".git")); err == nil {
		// Repository exists, open it
		repo, err := git.PlainOpen(c.localPath)
		if err != nil {
			return fmt.Errorf("failed to open existing repository: %w", err)
		}

		// Pull latest changes
		worktree, err := repo.Worktree()
		if err != nil {
			return fmt.Errorf("failed to get worktree: %w", err)
		}

		err = worktree.Pull(&git.PullOptions{
			RemoteName: "origin",
			Auth:       c.auth,
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return fmt.Errorf("failed to pull latest changes: %w", err)
		}

		return nil
	}

	// Clone the repository
	_, err := git.PlainClone(c.localPath, false, &git.CloneOptions{
		URL:           c.repoURL,
		Auth:          c.auth,
		ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", c.branch)),
		SingleBranch:  true,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	return nil
}

// CommitCluster commits cluster configuration files to the repository
func (c *Client) CommitCluster(clusterName string, files map[string]string) (string, error) {
	// Ensure repository is initialized
	if err := c.Initialize(); err != nil {
		return "", err
	}

	// Open repository
	repo, err := git.PlainOpen(c.localPath)
	if err != nil {
		return "", fmt.Errorf("failed to open repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	// Create cluster directory
	clusterDir := filepath.Join(c.localPath, "clusters", clusterName)
	if err := os.MkdirAll(clusterDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cluster directory: %w", err)
	}

	// Write files to cluster directory
	for filename, content := range files {
		filePath := filepath.Join(clusterDir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("failed to write file %s: %w", filename, err)
		}

		// Stage the file
		relativePath := filepath.Join("clusters", clusterName, filename)
		if _, err := worktree.Add(relativePath); err != nil {
			return "", fmt.Errorf("failed to stage file %s: %w", relativePath, err)
		}
	}

	// Commit changes
	commitMsg := fmt.Sprintf("Add cluster configuration for %s\n\nAutomated commit by OPL Cluster Operator", clusterName)
	commit, err := worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  c.authorName,
			Email: c.authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to commit changes: %w", err)
	}

	// Push to remote
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       c.auth,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", c.branch, c.branch)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to push changes: %w", err)
	}

	return commit.String(), nil
}

// DeleteCluster removes cluster configuration from the repository
func (c *Client) DeleteCluster(clusterName string) error {
	// Ensure repository is initialized
	if err := c.Initialize(); err != nil {
		return err
	}

	// Open repository
	repo, err := git.PlainOpen(c.localPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Remove cluster directory
	clusterDir := filepath.Join(c.localPath, "clusters", clusterName)
	if err := os.RemoveAll(clusterDir); err != nil {
		return fmt.Errorf("failed to remove cluster directory: %w", err)
	}

	// Stage the deletion
	relativePath := filepath.Join("clusters", clusterName)
	if _, err := worktree.Remove(relativePath); err != nil {
		return fmt.Errorf("failed to stage deletion: %w", err)
	}

	// Commit changes
	commitMsg := fmt.Sprintf("Remove cluster configuration for %s\n\nAutomated commit by OPL Cluster Operator", clusterName)
	_, err = worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  c.authorName,
			Email: c.authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit deletion: %w", err)
	}

	// Push to remote
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       c.auth,
	})
	if err != nil {
		return fmt.Errorf("failed to push deletion: %w", err)
	}

	return nil
}
