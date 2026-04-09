package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/ghas-projects/sarif-sql/internal/models"
)

// ParseRepositoriesFromFile reads repositories from a file (TOML or JSON)
func ParseRepositoriesFromFile(filePath string) ([]models.Repository, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".toml":
		return parseToml(data)
	case ".json":
		return parseJson(data)
	default:
		return nil, fmt.Errorf("unsupported file format: %s (expected .toml or .json)", ext)
	}
}

// parseToml parses TOML data into repositories
func parseToml(data []byte) ([]models.Repository, error) {
	// TOML structure uses [[repositories]] which creates a map with "repositories" key
	var wrapper struct {
		Repositories []models.Repository `toml:"repositories"`
	}
	if err := toml.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	return wrapper.Repositories, nil
}

// parseJson parses JSON data into repositories
func parseJson(data []byte) ([]models.Repository, error) {
	var repoList []models.Repository
	if err := json.Unmarshal(data, &repoList); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return repoList, nil
}

// ParseRepositoriesFromString parses comma-separated repository strings (owner/repo)
func ParseRepositoriesFromString(input string) ([]models.Repository, error) {
	if input == "" {
		return nil, fmt.Errorf("repository string cannot be empty")
	}

	repos := []models.Repository{}
	parts := strings.Split(input, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		segments := strings.Split(part, "/")
		if len(segments) != 2 {
			return nil, fmt.Errorf("invalid repository format: %s (expected owner/name)", part)
		}

		owner := strings.TrimSpace(segments[0])
		name := strings.TrimSpace(segments[1])

		if owner == "" || name == "" {
			return nil, fmt.Errorf("invalid repository format: %s (owner and name cannot be empty)", part)
		}

		repos = append(repos, models.Repository{
			FullName: owner + "/" + name,
		})
	}

	return repos, nil
}
