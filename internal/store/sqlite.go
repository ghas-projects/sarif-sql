package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghas-projects/sarif-protobuf/internal/models"
	_ "modernc.org/sqlite"
)

// SQLiteStore manages writing analysis results to a SQLite database.
type SQLiteStore struct {
	db     *sql.DB
	dbPath string
}

// NewSQLiteStore creates a new SQLite database at the given directory,
// initialises the schema, and returns a ready-to-use store.
func NewSQLiteStore(outputDir string) (*SQLiteStore, error) {
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	dbPath := filepath.Join(outputDir, "mrva-analysis.db")
	_ = os.Remove(dbPath) // start fresh

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}

	s := &SQLiteStore{db: db, dbPath: dbPath}
	if err := s.createSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error { return s.db.Close() }

// Path returns the filesystem path to the database file.
func (s *SQLiteStore) Path() string { return s.dbPath }

// BeginTx starts a new database transaction.
func (s *SQLiteStore) BeginTx() (*sql.Tx, error) {
	return s.db.Begin()
}

// ---------- schema ----------

func (s *SQLiteStore) createSchema() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS analysis (
	row_id                   INTEGER PRIMARY KEY,
	tool_name                TEXT NOT NULL,
	tool_version             TEXT,
	analysis_id              TEXT NOT NULL,
	controller_repo          TEXT,
	date                     TEXT,
	state                    TEXT NOT NULL,
	query_language           TEXT NOT NULL,
	created_at               TEXT NOT NULL,
	completed_at             TEXT,
	status                   TEXT NOT NULL,
	failure_reason           TEXT,
	scanned_repos_count      INTEGER NOT NULL,
	skipped_repos_count      INTEGER NOT NULL,
	not_found_repos_count    INTEGER NOT NULL,
	no_codeql_db_repos_count INTEGER NOT NULL,
	over_limit_repos_count   INTEGER NOT NULL,
	actions_workflow_run_id  INTEGER NOT NULL,
	total_repos_count        INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS repository (
	row_id                  INTEGER PRIMARY KEY,
	repository_full_name    TEXT NOT NULL,
	repository_url          TEXT NOT NULL,
	analysis_status         TEXT NOT NULL,
	result_count            INTEGER,
	artifact_size_in_bytes  INTEGER,
	analysis_id             TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rule (
	row_id           INTEGER PRIMARY KEY,
	id               TEXT NOT NULL,
	rule_name        TEXT NOT NULL,
	rule_description TEXT,
	property_tags    TEXT,
	kind             TEXT NOT NULL,
	severity_level   TEXT
);

CREATE TABLE IF NOT EXISTS alert (
	row_id               INTEGER PRIMARY KEY,
	file_path            TEXT NOT NULL,
	start_line           INTEGER,
	start_column         INTEGER,
	end_line             INTEGER,
	end_column           INTEGER,
	code_snippet_source  TEXT,
	code_snippet_sink    TEXT,
	code_snippet         TEXT,
	code_snippet_context TEXT,
	message              TEXT NOT NULL,
	result_fingerprint   TEXT,
	step_count           INTEGER,
	repository_row_id    INTEGER NOT NULL REFERENCES repository(row_id),
	analysis_row_id      INTEGER NOT NULL REFERENCES analysis(row_id),
	rule_row_id          INTEGER NOT NULL REFERENCES rule(row_id)
);`
	_, err := s.db.Exec(ddl)
	return err
}

// ---------- bulk write ----------

// WriteAnalysis inserts a single analysis row.
func (s *SQLiteStore) WriteAnalysis(tx *sql.Tx, a *models.Analysis) error {
	const q = `INSERT INTO analysis (
		row_id, tool_name, tool_version, analysis_id, controller_repo, date,
		state, query_language, created_at, completed_at, status, failure_reason,
		scanned_repos_count, skipped_repos_count, not_found_repos_count,
		no_codeql_db_repos_count, over_limit_repos_count, actions_workflow_run_id,
		total_repos_count
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

	_, err := tx.Exec(q,
		a.RowId, a.ToolName, optStr(a.ToolVersion), a.AnalysisId,
		optStr(a.ControllerRepo), optStr(a.Date),
		a.State, a.QueryLanguage, a.CreatedAt,
		optStr(a.CompletedAt), a.Status, optStr(a.FailureReason),
		a.ScannedReposCount, a.SkippedReposCount, a.NotFoundReposCount,
		a.NoCodeqlDbReposCount, a.OverLimitReposCount, a.ActionsWorkflowRunId,
		a.TotalReposCount,
	)
	return err
}

// WriteRepositories batch-inserts repositories using a prepared statement.
func (s *SQLiteStore) WriteRepositories(tx *sql.Tx, repos []*models.SQLRepository) error {
	stmt, err := tx.Prepare(`INSERT INTO repository (
		row_id, repository_full_name, repository_url, analysis_status,
		result_count, artifact_size_in_bytes, analysis_id
	) VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range repos {
		if _, err := stmt.Exec(
			r.RowId, r.RepositoryFullName, r.RepositoryUrl, r.AnalysisStatus,
			optInt32(r.ResultCount), optInt32(r.ArtifactSizeInBytes), r.AnalysisId,
		); err != nil {
			return fmt.Errorf("repository %s: %w", r.RepositoryFullName, err)
		}
	}
	return nil
}

// WriteRules batch-inserts rules using a prepared statement.
func (s *SQLiteStore) WriteRules(tx *sql.Tx, rules []*models.Rule) error {
	stmt, err := tx.Prepare(`INSERT INTO rule (
		row_id, id, rule_name, rule_description, property_tags, kind, severity_level
	) VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range rules {
		var tags *string
		if len(r.PropertyTags) > 0 {
			joined := strings.Join(r.PropertyTags, ",")
			tags = &joined
		}
		if _, err := stmt.Exec(
			r.RowId, r.Id, r.RuleName, optStr(r.RuleDescription),
			tags, r.Kind, optStr(r.SeverityLevel),
		); err != nil {
			return fmt.Errorf("rule %s: %w", r.Id, err)
		}
	}
	return nil
}

// WriteAlerts batch-inserts alerts using a prepared statement.
func (s *SQLiteStore) WriteAlerts(tx *sql.Tx, alerts []*models.Alert) error {
	stmt, err := tx.Prepare(`INSERT INTO alert (
		row_id, file_path, start_line, start_column, end_line, end_column,
		code_snippet_source, code_snippet_sink, code_snippet, code_snippet_context,
		message, result_fingerprint, step_count,
		repository_row_id, analysis_row_id, rule_row_id
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range alerts {
		if _, err := stmt.Exec(
			a.RowId, a.FilePath,
			optInt32(a.StartLine), optInt32(a.StartColumn),
			optInt32(a.EndLine), optInt32(a.EndColumn),
			optStr(a.CodeSnippetSource), optStr(a.CodeSnippetSink),
			optStr(a.CodeSnippet), optStr(a.CodeSnippetContext),
			a.Message, optStr(a.ResultFingerprint), optInt32(a.StepCount),
			a.RepositoryRowId, a.AnalysisRowId, a.RuleRowId,
		); err != nil {
			return fmt.Errorf("alert %d: %w", a.RowId, err)
		}
	}
	return nil
}

// ---------- nullable helpers ----------

// optStr converts an optional string pointer to a sql.NullString
// suitable for nullable TEXT columns.
func optStr(p *string) sql.NullString {
	if p == nil || *p == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

// optInt32 converts an optional int32 pointer to a sql.NullInt32
// suitable for nullable INTEGER columns.
func optInt32(p *int32) sql.NullInt32 {
	if p == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *p, Valid: true}
}
