package models

// Analysis represents a single MRVA analysis run.
type Analysis struct {
	RowId                int32   `json:"row_id"`
	ToolName             string  `json:"tool_name"`
	ToolVersion          *string `json:"tool_version,omitempty"`
	AnalysisId           string  `json:"analysis_id"`
	ControllerRepo       *string `json:"controller_repo,omitempty"`
	Date                 *string `json:"date,omitempty"`
	State                string  `json:"state"`
	QueryLanguage        string  `json:"query_language"`
	CreatedAt            string  `json:"created_at"`
	CompletedAt          *string `json:"completed_at,omitempty"`
	Status               string  `json:"status"`
	FailureReason        *string `json:"failure_reason,omitempty"`
	ScannedReposCount    int32   `json:"scanned_repos_count"`
	SkippedReposCount    int32   `json:"skipped_repos_count"`
	NotFoundReposCount   int32   `json:"not_found_repos_count"`
	NoCodeqlDbReposCount int32   `json:"no_codeql_db_repos_count"`
	OverLimitReposCount  int32   `json:"over_limit_repos_count"`
	ActionsWorkflowRunId int64   `json:"actions_workflow_run_id"`
	TotalReposCount      int32   `json:"total_repos_count"`
}

// SQLRepository represents a repository that was included in an analysis run.
type SQLRepository struct {
	RowId               int32  `json:"row_id"`
	RepositoryFullName  string `json:"repository_full_name"`
	RepositoryUrl       string `json:"repository_url"`
	AnalysisStatus      string `json:"analysis_status"`
	ResultCount         *int32 `json:"result_count,omitempty"`
	ArtifactSizeInBytes *int32 `json:"artifact_size_in_bytes,omitempty"`
	AnalysisId          string `json:"analysis_id"`
}

// Rule represents a CodeQL rule extracted from SARIF results.
type Rule struct {
	RowId           int32    `json:"row_id"`
	Id              string   `json:"id"`
	RuleName        string   `json:"rule_name"`
	RuleDescription *string  `json:"rule_description,omitempty"`
	PropertyTags    []string `json:"property_tags,omitempty"`
	Kind            string   `json:"kind"`
	SeverityLevel   *string  `json:"severity_level,omitempty"`
}

// Alert represents a single code-scanning alert extracted from SARIF results.
type Alert struct {
	RowId              int32   `json:"row_id"`
	FilePath           string  `json:"file_path"`
	StartLine          *int32  `json:"start_line,omitempty"`
	StartColumn        *int32  `json:"start_column,omitempty"`
	EndLine            *int32  `json:"end_line,omitempty"`
	EndColumn          *int32  `json:"end_column,omitempty"`
	CodeSnippetSource  *string `json:"code_snippet_source,omitempty"`
	CodeSnippetSink    *string `json:"code_snippet_sink,omitempty"`
	CodeSnippet        *string `json:"code_snippet,omitempty"`
	CodeSnippetContext *string `json:"code_snippet_context,omitempty"`
	Message            string  `json:"message"`
	ResultFingerprint  *string `json:"result_fingerprint,omitempty"`
	StepCount          *int32  `json:"step_count,omitempty"`
	RepositoryRowId    int32   `json:"repository_row_id"`
	RuleRowId          int32   `json:"rule_row_id"`
}
