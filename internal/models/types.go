package models

// Repository represents a GitHub repository to be analyzed
type Repository struct {
	FullName string `json:"full_name" toml:"full_name"`
}

// RepositoryList represents a collection of repositories
type RepositoryList struct {
	Repositories []Repository `json:"repositories" toml:"repositories"`
}
