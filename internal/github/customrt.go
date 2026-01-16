package github

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ghas-projects/sarif-avro/internal/auth"
	"github.com/ghas-projects/sarif-avro/internal/models"
)

// AuthProvider fetches an Authorization header value (e.g. "Bearer <token>") for a request.
// It may consult context, request, refresh tokens, etc. If it returns "", no Authorization header is set.
// If it returns an error the RoundTrip will return that error.
type AuthProvider func(req *http.Request) (authHeaderValue string, err error)

// Options controls the behavior of the CustomRoundTripper.
type Options struct {
	// Underlying transport to call. If nil, http.DefaultTransport is used.
	Base http.RoundTripper

	// Static headers to add to every request (GitHub-style headers or others).
	// Values will be set on req.Header (overwrites any existing header with same name).
	StaticHeaders map[string]string

	// Optional function called to provide Authorization header per-request.
	AuthProvider AuthProvider

	// Logger used for structured logging. If nil, slog.Default() is used.
	Logger *slog.Logger

	// Maximum number of bytes to log for request and response bodies.
	// Set to 0 to disable body logging.
	MaxBodyLogBytes int64
}

// tokenCache holds cached tokens by target type
type tokenCache struct {
	sync.RWMutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	token   string
	expires time.Time
}

var globalTokenCache = &tokenCache{
	tokens: make(map[string]cachedToken),
}

// CustomRoundTripper implements http.RoundTripper
type CustomRoundTripper struct {
	base            http.RoundTripper
	staticHeaders   map[string]string
	authProvider    AuthProvider
	logger          *slog.Logger
	maxBodyLogBytes int64
}

// NewCustomRoundTripper constructs a CustomRoundTripper with sane defaults.
func NewCustomRoundTripper(opts Options) *CustomRoundTripper {
	base := opts.Base
	if base == nil {
		base = http.DefaultTransport
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// copy static headers to avoid mutation later
	static := map[string]string{}
	for k, v := range opts.StaticHeaders {
		static[k] = v
	}

	return &CustomRoundTripper{
		base:            base,
		staticHeaders:   static,
		authProvider:    opts.AuthProvider,
		logger:          logger,
		maxBodyLogBytes: opts.MaxBodyLogBytes,
	}
}

// RoundTrip implements the http.RoundTripper interface.
func (c *CustomRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	// Create a shallow clone of request to avoid mutating caller's request headers/body
	req2 := req.Clone(req.Context())

	// Inject static headers (e.g., GitHub headers)
	for k, v := range c.staticHeaders {
		req2.Header.Set(k, v)
	}

	// Inject auth header if provider present
	if c.authProvider != nil {
		val, err := c.authProvider(req2)
		if err != nil {
			c.logger.Error("auth provider error", slog.String("method", req2.Method), slog.String("url", req2.URL.String()), slog.Any("error", err))
			return nil, err
		}
		if val != "" {
			req2.Header.Set("Authorization", val)
		}
	}

	c.logger.Info("HTTP Request",
		slog.String("method", req2.Method),
		slog.String("url", req2.URL.String()),
	)

	// Perform the actual request
	resp, err := c.base.RoundTrip(req2)
	duration := time.Since(start)

	if err != nil {
		c.logger.Error("HTTP Error",
			slog.String("method", req2.Method),
			slog.String("url", req2.URL.String()),
			slog.Any("error", err),
			slog.Duration("took", duration),
		)
		return nil, err
	}

	c.logger.Info("HTTP Response",
		slog.Int("status", resp.StatusCode),
		slog.String("method", req2.Method),
		slog.String("url", req2.URL.String()),
		slog.Duration("took", duration),
	)

	return resp, nil
}

// newGithubStyleTransportWithAuth creates a transport that injects GitHub headers and acquires token automatically.
// Internal helper that accepts explicit auth config.
func newGithubStyleTransportWithAuth(ctx context.Context, authConfig *auth.AuthConfig, logger *slog.Logger, targetInfo ...string) *CustomRoundTripper {
	static := map[string]string{
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
	}

	authProv := func(req *http.Request) (string, error) {
		// Check if using PAT token
		if token := authConfig.Token; token != "" {
			return "Bearer " + token, nil
		}

		// Using GitHub App authentication
		targetType := ""
		orgName := ""

		if len(targetInfo) > 0 {
			targetType = targetInfo[0]
		}
		if len(targetInfo) > 1 {
			orgName = targetInfo[1]
		}

		cacheKey := targetType
		if orgName != "" {
			cacheKey = targetType + ":" + orgName
		}

		globalTokenCache.RLock()
		if cached, ok := globalTokenCache.tokens[cacheKey]; ok && time.Now().Before(cached.expires) {
			token := cached.token
			globalTokenCache.RUnlock()
			return "Bearer " + token, nil
		}
		globalTokenCache.RUnlock()

		globalTokenCache.Lock()
		defer globalTokenCache.Unlock()

		// Double-check after acquiring write lock
		if cached, ok := globalTokenCache.tokens[cacheKey]; ok && time.Now().Before(cached.expires) {
			return "Bearer " + cached.token, nil
		}

		ts := auth.NewTokenService(
			authConfig.AppID,
			authConfig.PrivateKey,
			authConfig.BaseURL,
		)

		var tokenStr string
		var err error

		if targetType == models.OrganizationType {
			if orgName != "" {
				tokenStr, err = ts.GetInstallationTokenForOrg(orgName)
				if err != nil {
					return "", err
				}
			} else {
				token, err := ts.GetInstallationToken(targetType)
				if err != nil {
					return "", err
				}
				tokenStr = token.Token
			}
		} else {
			token, err := ts.GetInstallationToken(targetType)
			if err != nil {
				return "", err
			}
			tokenStr = token.Token
		}

		// Cache the token for 55 minutes
		globalTokenCache.tokens[cacheKey] = cachedToken{
			token:   tokenStr,
			expires: time.Now().Add(55 * time.Minute),
		}

		return "Bearer " + tokenStr, nil
	}

	return NewCustomRoundTripper(Options{
		Base:          http.DefaultTransport,
		StaticHeaders: static,
		AuthProvider:  authProv,
		Logger:        logger,
	})
}

// NewGithubStyleTransport creates a transport using the global auth.Auth configuration.
// Deprecated: Use GetAuthenticatedTransport with explicit auth config instead.
func NewGithubStyleTransport(ctx context.Context, logger *slog.Logger, targetInfo ...string) *CustomRoundTripper {
	return newGithubStyleTransportWithAuth(ctx, auth.Auth, logger, targetInfo...)
}

// GetAuthenticatedTransport returns the appropriate GitHub transport based on auth configuration.
// For GitHub App authentication, you can optionally provide a repository string (owner/repo format)
// to extract the organization name. This is the recommended API for creating authenticated transports.
func GetAuthenticatedTransport(ctx context.Context, authConfig *auth.AuthConfig, logger *slog.Logger, repo ...string) *CustomRoundTripper {
	// Check if using PAT authentication
	if authConfig.Token != "" {
		return newGithubStyleTransportWithAuth(ctx, authConfig, logger)
	}

	// Using GitHub App authentication
	if authConfig.AppID != "" && authConfig.PrivateKey != "" {
		// If a repository string is provided (owner/repo format), extract the owner as org name
		if len(repo) > 0 && repo[0] != "" && strings.Contains(repo[0], "/") {
			parts := strings.Split(repo[0], "/")
			orgName := parts[0]
			return newGithubStyleTransportWithAuth(ctx, authConfig, logger, models.OrganizationType, orgName)
		}
		// Fall back to generic organization type without specific org name
		return newGithubStyleTransportWithAuth(ctx, authConfig, logger, models.OrganizationType)
	}

	// Default fallback (no auth)
	return newGithubStyleTransportWithAuth(ctx, authConfig, logger)
}
