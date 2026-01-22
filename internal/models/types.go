package models

// Repository represents a GitHub repository to be analyzed
type Repository struct {
	FullName string `json:"full_name" toml:"full_name"`
}

type ScannedRepository struct {
	Repository        Repository `json:"repository"`
	AnalysisStatus    string     `json:"analysis_status"`
	ResultCount       int        `json:"result_count"`
	ArtifactSizeBytes int64      `json:"artifact_size_in_bytes"`
}

type AccessMismatchRepositories struct {
	RepositoryCount int          `json:"repository_count"`
	Repositories    []Repository `json:"repositories"`
}

type SkippedRepositories struct {
	AccessMismatchRepositories AccessMismatchRepositories `json:"access_mismatch_repos"`
}

type NotFoundRepositories struct {
	RepositoryCount int      `json:"repository_count"`
	Repositories    []string `json:"repository_full_names"`
}

type NoCodeQLDBRepositories struct {
	RepositoryCount int          `json:"repository_count"`
	Repositories    []Repository `json:"repositories"`
}

type OverLimitRepositories struct {
	RepositoryCount int          `json:"repository_count"`
	Repositories    []Repository `json:"repositories"`
}

type MRVAStatusResponse struct {
	Repository        Repository `json:"repository"`
	AnalysisStatus    string     `json:"analysis_status"`
	ArtifactSizeBytes int64      `json:"artifact_size_in_bytes"`
	ResultCount       int        `json:"result_count"`
	DatabaseCommitSHA string     `json:"database_commit_sha"`
	ArtifactURL       string     `json:"artifact_url"`
	SarifFilePath     string
}

type MRVASummaryResponse struct {
	ControllerRepo         Repository             `json:"controller_repo"`
	QueryLanguage          string                 `json:"query_language"`
	QueryPackURL           string                 `json:"query_pack_url"`
	CompletedAt            string                 `json:"completed_at"`
	CreatedAt              string                 `json:"created_at"`
	ActionsWorkflowRunID   int                    `json:"actions_workflow_run_id"`
	ScannedRepositories    []ScannedRepository    `json:"scanned_repositories"`
	SkippedRepositories    SkippedRepositories    `json:"skipped_repositories"`
	NotFoundRepositories   NotFoundRepositories   `json:"not_found_repositories"`
	NoCodeQLDBRepositories NoCodeQLDBRepositories `json:"no_codeql_db_repositories"`
	OverLimitRepositories  OverLimitRepositories  `json:"over_limit_repositories"`
}
