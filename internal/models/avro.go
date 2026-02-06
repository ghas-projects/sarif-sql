package models

// AvroRepository represents the Avro schema for Repository
type AvroRepository struct {
	RepositoryID   int    `avro:"repository_id" json:"repository_id"`
	RepositoryName string `avro:"repository_name" json:"repository_name"`
	RepositoryURL  string `avro:"repository_url" json:"repository_url"`
}

// AvroRun represents the Avro schema for Run
type AvroRun struct {
	RunID          int     `avro:"run_id" json:"run_id"`
	ToolName       string  `avro:"tool_name" json:"tool_name"`
	ToolVersion    *string `avro:"tool_version" json:"tool_version,omitempty"`
	AnalysisID     string  `avro:"analysis_id" json:"analysis_id"`
	ControllerRepo *string `avro:"controller_repo" json:"controller_repo,omitempty"`
	Date           *string `avro:"date" json:"date,omitempty"`
}

// AvroRule represents the Avro schema for Rule
type AvroRule struct {
	RuleID          int      `avro:"rule_id" json:"rule_id"`
	ID              string   `avro:"id" json:"id"`
	RuleName        string   `avro:"rule_name" json:"rule_name"`
	RuleDescription *string  `avro:"rule_description" json:"rule_description,omitempty"`
	PropertyTags    []string `avro:"property_tags" json:"property_tags,omitempty"`
	Kind            string   `avro:"kind" json:"kind"`
	SeverityLevel   *string  `avro:"severity_level" json:"problem.severity,omitempty"`
}

// AvroAlert represents the Avro schema for Alert
type AvroAlert struct {
	AlertID            int     `avro:"alert_id" json:"alert_id"`
	FilePath           string  `avro:"file_path" json:"file_path"`
	StartLine          *int    `avro:"start_line" json:"start_line,omitempty"`
	StartColumn        *int    `avro:"start_column" json:"start_column,omitempty"`
	EndLine            *int    `avro:"end_line" json:"end_line,omitempty"`
	EndColumn          *int    `avro:"end_column" json:"end_column,omitempty"`
	CodeSnippetSource  *string `avro:"code_snippet_source" json:"code_snippet_source,omitempty"`
	CodeSnippetSink    *string `avro:"code_snippet_sink" json:"code_snippet_sink,omitempty"`
	CodeSnippet        *string `avro:"code_snippet" json:"code_snippet,omitempty"`
	CodeSnippetContext *string `avro:"code_snippet_context" json:"code_snippet_context,omitempty"`
	ResultFingerprint  *string `avro:"result_fingerprint" json:"result_fingerprint,omitempty"`
	StepCount          *int    `avro:"step_count" json:"step_count,omitempty"`
	RepositoryID       int     `avro:"repository_id" json:"repository_id"`
	AnalysisID         int     `avro:"analysis_id" json:"analysis_id"`
	RuleID             int     `avro:"rule_id" json:"rule_id"`
	Message            string  `avro:"message" json:"message"`
}

type AvroResult struct {
	AvroAlerts     []AvroAlert      `avro:"alerts" json:"alerts"`
	AvroRun        AvroRun          `avro:"run" json:"run"`
	AvroRepository []AvroRepository `avro:"repository" json:"repository"`
	AvroRules      []AvroRule       `avro:"rules" json:"rules"`
}
