/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package database

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Client handles PostgreSQL database operations
type Client struct {
	pool   *pgxpool.Pool
	config Config
}

// Config holds database connection configuration
type Config struct {
	Host            string
	Port            int
	Database        string
	Username        string
	Password        string
	SSLMode         string // disable, require, verify-ca, verify-full
	MaxConns        int
	MinConns        int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// NewClient creates a new database client
func NewClient(cfg Config) (*Client, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("database host is required")
	}
	if cfg.Database == "" {
		return nil, fmt.Errorf("database name is required")
	}
	if cfg.Username == "" {
		return nil, fmt.Errorf("database username is required")
	}

	// Set defaults
	if cfg.Port == 0 {
		cfg.Port = 5432
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "require"
	}
	if cfg.MaxConns == 0 {
		cfg.MaxConns = 10
	}
	if cfg.MinConns == 0 {
		cfg.MinConns = 2
	}
	if cfg.ConnMaxLifetime == 0 {
		cfg.ConnMaxLifetime = 1 * time.Hour
	}
	if cfg.ConnMaxIdleTime == 0 {
		cfg.ConnMaxIdleTime = 10 * time.Minute
	}

	return &Client{
		config: cfg,
		pool:   nil,
	}, nil
}

// Initialize establishes the connection pool
func (c *Client) Initialize(ctx context.Context) error {
	// Build connection string with properly escaped credentials
	// URL-encode username and password to handle special characters
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(c.config.Username),
		url.QueryEscape(c.config.Password),
		c.config.Host,
		c.config.Port,
		url.QueryEscape(c.config.Database),
		c.config.SSLMode,
	)

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return fmt.Errorf("invalid database config: %w", err)
	}

	poolConfig.MaxConns = int32(c.config.MaxConns)
	poolConfig.MinConns = int32(c.config.MinConns)
	poolConfig.MaxConnLifetime = c.config.ConnMaxLifetime
	poolConfig.MaxConnIdleTime = c.config.ConnMaxIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	c.pool = pool
	return nil
}

// InitializeWithRetry establishes connection with retry logic
func (c *Client) InitializeWithRetry(ctx context.Context, maxRetries int) error {
	backoff := 1 * time.Second
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		err := c.Initialize(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		if i < maxRetries-1 {
			// Don't sleep on last iteration if it fails
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
			}
		}
	}

	return fmt.Errorf("failed to connect after %d retries, last error: %w", maxRetries, lastErr)
}

// Close closes the connection pool
func (c *Client) Close() {
	if c.pool != nil {
		c.pool.Close()
		c.pool = nil
	}
}

// Ping checks database connectivity
func (c *Client) Ping(ctx context.Context) error {
	if c.pool == nil {
		return fmt.Errorf("connection pool not initialized")
	}
	return c.pool.Ping(ctx)
}

// IsInitialized returns true if the connection pool is initialized
func (c *Client) IsInitialized() bool {
	return c.pool != nil
}
