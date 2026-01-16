package models

// Repository represents a GitHub repository to be analyzed
type Repository struct {
	FullName string `json:"full_name" toml:"full_name"`
}

// RepositoryList represents a collection of repositories
type RepositoryList struct {
	Repositories []Repository `json:"repositories" toml:"repositories"`
}

type MRVAStatusResponse struct {
	Repository        Repository `json:"repository"`
	AnalysisStatus    string     `json:"analysis_status"`
	ArtifactSizeBytes int64      `json:"artifact_size_in_bytes"`
	ResultCount       int        `json:"result_count"`
	DatabaseCommitSHA string     `json:"database_commit_sha"`
	ArtifactURL       string     `json:"artifact_url"`
}
