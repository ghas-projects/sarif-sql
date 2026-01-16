package models

import "log/slog"

// Logger is the global logger instance initialized by the root command
var Logger *slog.Logger

const (
	DefaultBaseURL   string = "https://api.github.com"
	EnterpriseType   string = "Enterprise"
	OrganizationType string = "Organization"
)
