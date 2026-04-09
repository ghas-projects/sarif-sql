package github

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/ghas-projects/sarif-sql/internal/auth"
)

// Client is a GitHub API client that manages HTTP connections and authentication
type Client struct {
	httpClient *http.Client
	auth       *auth.AuthConfig
	logger     *slog.Logger
	baseURL    string
}

// NewClient creates a new GitHub API client with connection reuse
func NewClient(auth *auth.AuthConfig, logger *slog.Logger) *Client {
	transport := GetAuthenticatedTransport(context.Background(), auth, logger)
	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   0, // Use context timeouts per request
		},
		auth:    auth,
		logger:  logger,
		baseURL: auth.BaseURL,
	}
}
