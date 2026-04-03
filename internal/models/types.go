package models

import (
	"sync/atomic"
	"time"
)

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
	ID                     int                    `json:"id"`
	ControllerRepo         Repository             `json:"controller_repo"`
	QueryLanguage          string                 `json:"query_language"`
	QueryPackURL           string                 `json:"query_pack_url"`
	Status                 string                 `json:"status"`
	FailureReason          string                 `json:"failure_reason,omitempty"`
	CompletedAt            string                 `json:"completed_at"`
	CreatedAt              string                 `json:"created_at"`
	ActionsWorkflowRunID   int64                  `json:"actions_workflow_run_id"`
	ScannedRepositories    []ScannedRepository    `json:"scanned_repositories"`
	SkippedRepositories    SkippedRepositories    `json:"skipped_repositories"`
	NotFoundRepositories   NotFoundRepositories   `json:"not_found_repositories"`
	NoCodeQLDBRepositories NoCodeQLDBRepositories `json:"no_codeql_db_repositories"`
	OverLimitRepositories  OverLimitRepositories  `json:"over_limit_repositories"`
}

// AnalysisRecord represents the Analysis proto model as a flat JSON-serializable struct
type AnalysisRecord struct {
	RowID                int32  `json:"row_id"`
	ToolName             string `json:"tool_name"`
	ToolVersion          string `json:"tool_version,omitempty"`
	AnalysisID           string `json:"analysis_id"`
	ControllerRepo       string `json:"controller_repo,omitempty"`
	Date                 string `json:"date,omitempty"`
	State                string `json:"state"`
	QueryLanguage        string `json:"query_language"`
	CreatedAt            string `json:"created_at"`
	CompletedAt          string `json:"completed_at,omitempty"`
	Status               string `json:"status"`
	FailureReason        string `json:"failure_reason,omitempty"`
	ScannedReposCount    int32  `json:"scanned_repos_count"`
	SkippedReposCount    int32  `json:"skipped_repos_count"`
	NotFoundReposCount   int32  `json:"not_found_repos_count"`
	NoCodeqlDBReposCount int32  `json:"no_codeql_db_repos_count"`
	OverLimitReposCount  int32  `json:"over_limit_repos_count"`
	ActionsWorkflowRunID int64  `json:"actions_workflow_run_id"`
	TotalReposCount      int32  `json:"total_repos_count"`
}

// analysisRowIDCounter is a sequential counter for generating unique analysis row IDs.
var analysisRowIDCounter int64

// repositoryRowIDCounter is a sequential counter for generating unique repository row IDs.
var repositoryRowIDCounter int64

// ToAnalysisRecord converts the summary API response into a flat AnalysisRecord
// matching the Analysis proto model.
func (s *MRVASummaryResponse) ToAnalysisRecord(analysisID, controllerRepo string) AnalysisRecord {
	return AnalysisRecord{
		RowID:                int32(atomic.AddInt64(&analysisRowIDCounter, 1)),
		ToolName:             "CodeQL",
		AnalysisID:           analysisID,
		ControllerRepo:       controllerRepo,
		Date:                 time.Now().Format("2006-01-02 15:04:05"),
		State:                s.Status,
		QueryLanguage:        s.QueryLanguage,
		CreatedAt:            s.CreatedAt,
		CompletedAt:          s.CompletedAt,
		Status:               s.Status,
		FailureReason:        s.FailureReason,
		ScannedReposCount:    int32(len(s.ScannedRepositories)),
		SkippedReposCount:    int32(s.SkippedRepositories.AccessMismatchRepositories.RepositoryCount),
		NotFoundReposCount:   int32(s.NotFoundRepositories.RepositoryCount),
		NoCodeqlDBReposCount: int32(s.NoCodeQLDBRepositories.RepositoryCount),
		OverLimitReposCount:  int32(s.OverLimitRepositories.RepositoryCount),
		ActionsWorkflowRunID: s.ActionsWorkflowRunID,
		TotalReposCount: int32(len(s.ScannedRepositories)) +
			int32(s.SkippedRepositories.AccessMismatchRepositories.RepositoryCount) +
			int32(s.NotFoundRepositories.RepositoryCount) +
			int32(s.NoCodeQLDBRepositories.RepositoryCount) +
			int32(s.OverLimitRepositories.RepositoryCount),
	}
}

// RepositoryRecord represents the Repository proto model as a flat JSON-serializable struct
type RepositoryRecord struct {
	RowID               int32  `json:"row_id"`
	RepositoryFullName  string `json:"repository_full_name"`
	RepositoryURL       string `json:"repository_url"`
	AnalysisStatus      string `json:"analysis_status"`
	ResultCount         *int32 `json:"result_count,omitempty"`
	ArtifactSizeInBytes *int32 `json:"artifact_size_in_bytes,omitempty"`
	AnalysisID          string `json:"analysis_id"`
}

// ToRepositoryRecords converts all repositories in the MRVASummaryResponse into RepositoryRecord slices
// matching the Repository proto model. This includes scanned, skipped, not found, no CodeQL DB, and over limit repos.
func ToRepositoryRecords(summary MRVASummaryResponse, analysisID string) []RepositoryRecord {
	totalCap := len(summary.ScannedRepositories) +
		summary.SkippedRepositories.AccessMismatchRepositories.RepositoryCount +
		summary.NotFoundRepositories.RepositoryCount +
		summary.NoCodeQLDBRepositories.RepositoryCount +
		summary.OverLimitRepositories.RepositoryCount

	records := make([]RepositoryRecord, 0, totalCap)

	// Scanned repositories
	for _, scanned := range summary.ScannedRepositories {
		resultCount := int32(scanned.ResultCount)
		artifactSize := int32(scanned.ArtifactSizeBytes)
		records = append(records, RepositoryRecord{
			RowID:               int32(atomic.AddInt64(&repositoryRowIDCounter, 1)),
			RepositoryFullName:  scanned.Repository.FullName,
			RepositoryURL:       "https://github.com/" + scanned.Repository.FullName,
			AnalysisStatus:      scanned.AnalysisStatus,
			ResultCount:         &resultCount,
			ArtifactSizeInBytes: &artifactSize,
			AnalysisID:          analysisID,
		})
	}

	// Skipped repositories (access mismatch)
	for _, repo := range summary.SkippedRepositories.AccessMismatchRepositories.Repositories {
		records = append(records, RepositoryRecord{
			RowID:              int32(atomic.AddInt64(&repositoryRowIDCounter, 1)),
			RepositoryFullName: repo.FullName,
			RepositoryURL:      "https://github.com/" + repo.FullName,
			AnalysisStatus:     "access_mismatch",
			AnalysisID:         analysisID,
		})
	}

	// Not found repositories
	for _, fullName := range summary.NotFoundRepositories.Repositories {
		records = append(records, RepositoryRecord{
			RowID:              int32(atomic.AddInt64(&repositoryRowIDCounter, 1)),
			RepositoryFullName: fullName,
			RepositoryURL:      "https://github.com/" + fullName,
			AnalysisStatus:     "not_found",
			AnalysisID:         analysisID,
		})
	}

	// No CodeQL DB repositories
	for _, repo := range summary.NoCodeQLDBRepositories.Repositories {
		records = append(records, RepositoryRecord{
			RowID:              int32(atomic.AddInt64(&repositoryRowIDCounter, 1)),
			RepositoryFullName: repo.FullName,
			RepositoryURL:      "https://github.com/" + repo.FullName,
			AnalysisStatus:     "no_codeql_db",
			AnalysisID:         analysisID,
		})
	}

	// Over limit repositories
	for _, repo := range summary.OverLimitRepositories.Repositories {
		records = append(records, RepositoryRecord{
			RowID:              int32(atomic.AddInt64(&repositoryRowIDCounter, 1)),
			RepositoryFullName: repo.FullName,
			RepositoryURL:      "https://github.com/" + repo.FullName,
			AnalysisStatus:     "over_limit",
			AnalysisID:         analysisID,
		})
	}

	return records
}
