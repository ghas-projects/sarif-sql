package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ghas-projects/sarif-sql/internal/models"
	"github.com/ghas-projects/sarif-sql/util"
)

// ResultCollector provides thread-safe access to the master result structure
type ResultCollector struct {
	alertsMu sync.Mutex
	Alerts   []*models.Alert

	Analysis *models.Analysis

	reposMu      sync.RWMutex
	Repositories map[string]*models.SQLRepository

	rulesMu sync.RWMutex
	Rules   map[string]*models.Rule

	// Atomic counters for unique ID generation across goroutines
	alertCounter int64
	ruleCounter  int64
}

// NewResultCollector creates a new ResultCollector instance
func NewResultCollector() *ResultCollector {
	return &ResultCollector{
		Repositories: make(map[string]*models.SQLRepository),
		Rules:        make(map[string]*models.Rule),
		Alerts:       make([]*models.Alert, 0),
		Analysis:     &models.Analysis{},
	}
}

// AddAlerts adds multiple alerts in a single lock operation (more efficient)
func (s *ResultCollector) AddAlerts(alerts []*models.Alert) {
	s.alertsMu.Lock()
	s.Alerts = append(s.Alerts, alerts...)
	s.alertsMu.Unlock()
}

// TransformService handles SARIF transformation and data extraction
type TransformService struct {
	logger         *slog.Logger
	sarifDirPath   string
	outputDir      string
	analysisID     string
	controllerRepo string
}

// NewTransformService creates a new TransformService instance
func NewTransformService(logger *slog.Logger, sarifDirPath string, outputDir string, analysisID string, controllerRepo string) *TransformService {
	return &TransformService{
		logger:         logger,
		sarifDirPath:   sarifDirPath,
		outputDir:      outputDir,
		analysisID:     analysisID,
		controllerRepo: controllerRepo,
	}
}

// Transform converts SARIF files into structured result data
func (ts *TransformService) Transform(ctx context.Context) (result *ResultCollector, err error) {
	// Read all entries in the directory
	dirEntries, err := os.ReadDir(ts.sarifDirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SARIF directory: %w", err)
	}

	// Create the master structure that all workers will write to directly
	masterResult := NewResultCollector()

	// Load analysis metadata from analysis.json (written during download phase)
	analysisPath := filepath.Join(ts.sarifDirPath, "analysis.json")
	analysis, err := ts.loadAnalysisFromJSON(analysisPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load analysis.json: %w", err)
	}
	masterResult.Analysis = analysis
	ts.logger.Info("loaded analysis from analysis.json",
		"analysis_id", analysis.AnalysisId,
		"tool_name", analysis.ToolName)

	// Load repositories from repos.json (written during download phase)
	reposPath := filepath.Join(ts.sarifDirPath, "repos.json")
	repos, err := ts.loadRepositoriesFromJSON(reposPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load repos.json: %w", err)
	}
	for _, repo := range repos {
		// Normalize key to lowercase for case-insensitive lookup.
		// The GitHub API may return repository full names in different casing
		// than the canonical URI in SARIF versionControlProvenance.
		masterResult.Repositories[strings.ToLower(repo.RepositoryFullName)] = repo
	}
	ts.logger.Info("loaded repositories from repos.json",
		"count", len(repos))

	// Filter to only SARIF files (skip analysis.json, repos.json, etc.)
	var sarifFileNames []string
	for _, entry := range dirEntries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".sarif") {
			sarifFileNames = append(sarifFileNames, entry.Name())
		}
	}

	if len(sarifFileNames) == 0 {
		ts.logger.Warn("no SARIF files found in directory", "path", ts.sarifDirPath)
		return masterResult, nil
	}

	// Calculate optimal number of workers for concurrent processing
	workers := util.CalculateOptimalWorkers(len(sarifFileNames))

	ts.logger.Info("starting SARIF files processing",
		"analysis_id", ts.analysisID,
		"controller_repo", ts.controllerRepo,
		"sarif_file_count", len(sarifFileNames),
		"workers", workers)

	sarifChan := make(chan string, len(sarifFileNames))

	var wg sync.WaitGroup

	ts.logger.Info("spawning worker goroutines",
		"worker_count", workers)

	// Spawn workers that write directly to masterResult
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerId int) {
			defer wg.Done()
			ts.processSarifFiles(ctx, workerId, sarifChan, masterResult)
		}(i)
	}

	// Send only SARIF files to the channel
	for _, name := range sarifFileNames {
		sarifChan <- name
	}
	close(sarifChan)

	// Wait for all workers to complete
	wg.Wait()

	ts.logger.Info("transformation processing completed",
		"total_alerts", len(masterResult.Alerts),
		"total_runs", 1,
		"total_repositories", len(masterResult.Repositories),
		"total_rules", len(masterResult.Rules))

	return masterResult, nil
}

// processSarifFiles is the worker function that processes SARIF files and writes directly to the master structure
func (ts *TransformService) processSarifFiles(ctx context.Context, workerId int, sarifChan <-chan string, masterResult *ResultCollector) {
	for sarifFile := range sarifChan {
		// Check for cancellation
		select {
		case <-ctx.Done():
			ts.logger.Warn("worker cancelled",
				"worker_id", workerId,
				"reason", ctx.Err())
			return
		default:
			// Continue processing
		}

		ts.logger.Debug("worker processing SARIF file",
			"worker_id", workerId,
			"sarif_file", sarifFile)

		// Read SARIF file
		sarifPath := filepath.Join(ts.sarifDirPath, sarifFile)
		data, err := os.ReadFile(sarifPath)
		if err != nil {
			ts.logger.Error("failed to read SARIF file",
				"worker_id", workerId,
				"sarif_file", sarifFile,
				"error", err)
			continue
		}

		// Parse SARIF JSON
		var sarifDoc models.SarifDocument
		if err := json.Unmarshal(data, &sarifDoc); err != nil {
			ts.logger.Error("failed to unmarshal SARIF JSON",
				"worker_id", workerId,
				"sarif_file", sarifFile,
				"error", err)
			continue
		}

		ts.logger.Info("successfully read and parsed SARIF file",
			"worker_id", workerId,
			"sarif_file", sarifFile,
			"size_bytes", len(data))

		// Check if document has runs
		if len(sarifDoc.Runs) == 0 {
			ts.logger.Warn("no runs found in SARIF file",
				"worker_id", workerId,
				"sarif_file", sarifFile)
			continue
		}

		// Process each run
		for runIdx, run := range sarifDoc.Runs {

			// Look up repository from pre-loaded repos.json data (case-insensitive)
			repoFullName := ts.getRepoFullNameFromRun(run, sarifFile)
			var repoRowId int32
			masterResult.reposMu.RLock()
			if repo, found := masterResult.Repositories[strings.ToLower(repoFullName)]; found {
				repoRowId = repo.RowId
			} else {
				ts.logger.Warn("repository not found in repos.json",
					"worker_id", workerId,
					"repo_full_name", repoFullName,
					"sarif_file", sarifFile)
			}
			masterResult.reposMu.RUnlock()

			// Extract rules and write directly to master (with auto-deduplication)
			ruleMap := ts.extractRules(run, masterResult)

			// Extract alerts and write directly to master (pass repo ID and ruleMap for FK references)
			alerts := ts.extractAlerts(run, repoRowId, ruleMap, masterResult)
			masterResult.AddAlerts(alerts)

			ts.logger.Info("transformed SARIF run",
				"worker_id", workerId,
				"sarif_file", sarifFile,
				"run_index", runIdx,
				"alerts_count", len(alerts),
				"rules_count", len(ruleMap))
		}
	}

}

// loadAnalysisFromJSON reads analysis.json and converts it to an Analysis
func (ts *TransformService) loadAnalysisFromJSON(filePath string) (*models.Analysis, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	var record models.AnalysisRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filePath, err)
	}

	analysis := &models.Analysis{
		RowId:                record.RowID,
		ToolName:             record.ToolName,
		AnalysisId:           record.AnalysisID,
		State:                record.State,
		QueryLanguage:        record.QueryLanguage,
		CreatedAt:            record.CreatedAt,
		Status:               record.Status,
		ScannedReposCount:    record.ScannedReposCount,
		SkippedReposCount:    record.SkippedReposCount,
		NotFoundReposCount:   record.NotFoundReposCount,
		NoCodeqlDbReposCount: record.NoCodeqlDBReposCount,
		OverLimitReposCount:  record.OverLimitReposCount,
		ActionsWorkflowRunId: record.ActionsWorkflowRunID,
		TotalReposCount:      record.TotalReposCount,
	}

	// Set optional fields
	if record.ToolVersion != "" {
		analysis.ToolVersion = &record.ToolVersion
	}
	if record.ControllerRepo != "" {
		analysis.ControllerRepo = &record.ControllerRepo
	}
	if record.Date != "" {
		analysis.Date = &record.Date
	}
	if record.CompletedAt != "" {
		analysis.CompletedAt = &record.CompletedAt
	}
	if record.FailureReason != "" {
		analysis.FailureReason = &record.FailureReason
	}

	return analysis, nil
}

// loadRepositoriesFromJSON reads repos.json and converts it to a map of SQLRepository keyed by full name
func (ts *TransformService) loadRepositoriesFromJSON(filePath string) (map[string]*models.SQLRepository, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	var records []models.RepositoryRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filePath, err)
	}

	repos := make(map[string]*models.SQLRepository, len(records))
	for _, record := range records {
		repo := &models.SQLRepository{
			RowId:               record.RowID,
			RepositoryFullName:  record.RepositoryFullName,
			RepositoryUrl:       record.RepositoryURL,
			AnalysisStatus:      record.AnalysisStatus,
			ResultCount:         record.ResultCount,
			ArtifactSizeInBytes: record.ArtifactSizeInBytes,
			AnalysisId:          record.AnalysisID,
		}
		repos[record.RepositoryFullName] = repo
	}

	return repos, nil
}

// getRepoFullNameFromRun extracts the repository full name from a SARIF run's versionControlProvenance
func (ts *TransformService) getRepoFullNameFromRun(run models.SarifRun, sarifFile string) string {
	if len(run.VersionControlProvenance) > 0 {
		repoURI := strings.TrimRight(run.VersionControlProvenance[0].RepositoryURI, "/")
		if parts := strings.Split(repoURI, "/"); len(parts) >= 2 {
			return parts[len(parts)-2] + "/" + parts[len(parts)-1]
		}
	}
	return sarifFile // Fallback to filename
}

// extractRules extracts rules from SARIF run
func (ts *TransformService) extractRules(run models.SarifRun, masterResult *ResultCollector) map[string]*models.Rule {
	ruleMap := make(map[string]*models.Rule)

	for _, rule := range run.Tool.Driver.Rules {
		ruleStringID := rule.ID

		// Fast path: if rule already exists in master, reuse it and skip construction
		masterResult.rulesMu.RLock()
		existingRule, exists := masterResult.Rules[ruleStringID]
		masterResult.rulesMu.RUnlock()

		if exists {
			ruleMap[ruleStringID] = existingRule
			continue
		}

		// Build the rule fully before inserting into the shared map
		pbRule := &models.Rule{
			Id:       rule.ID,
			RuleName: rule.Name,
		}

		// Extract properties if present
		if rule.Properties != nil {
			if desc, ok := rule.Properties["description"].(string); ok {
				pbRule.RuleDescription = &desc
			}
			if kind, ok := rule.Properties["kind"].(string); ok {
				pbRule.Kind = kind
			}
			if tags, ok := rule.Properties["tags"].([]interface{}); ok {
				var tagStrings []string
				for _, tag := range tags {
					if tagStr, ok := tag.(string); ok {
						tagStrings = append(tagStrings, tagStr)
					}
				}
				pbRule.PropertyTags = tagStrings
			}
			if severity, ok := rule.Properties["problem.severity"].(string); ok {
				pbRule.SeverityLevel = &severity
			}
		}

		// Double-check under write lock (another goroutine may have inserted between RLock and Lock)
		masterResult.rulesMu.Lock()
		if existingRule, exists := masterResult.Rules[ruleStringID]; exists {
			// Another goroutine inserted it first — reuse existing
			pbRule.RowId = existingRule.RowId
		} else {
			// Generate new unique rule ID using atomic counter (1-based for primary key)
			pbRule.RowId = int32(atomic.AddInt64(&masterResult.ruleCounter, 1))
			// Insert fully-built rule so other goroutines see complete data
			masterResult.Rules[ruleStringID] = pbRule
		}
		masterResult.rulesMu.Unlock()

		ruleMap[pbRule.Id] = pbRule
	}

	return ruleMap
}

// extractAlerts extracts alerts from SARIF run
func (ts *TransformService) extractAlerts(run models.SarifRun, repositoryID int32, ruleMap map[string]*models.Rule, masterResult *ResultCollector) []*models.Alert {
	var alerts []*models.Alert

	for _, result := range run.Results {
		// Generate unique alert ID using atomic counter (1-based for primary key)
		alertID := int32(atomic.AddInt64(&masterResult.alertCounter, 1))

		pbAlert := &models.Alert{
			RowId:           alertID,
			AnalysisRowId:   masterResult.Analysis.RowId, // FK to Analysis from analysis.json
			RepositoryRowId: repositoryID,                // FK to Repository from repos.json
			Message:         result.Message.Text,
		}

		// Get Rule ID (convert string identifier to numeric PK)
		ruleIDStr := result.RuleID
		// First check local ruleMap
		if rule, found := ruleMap[ruleIDStr]; found {
			pbAlert.RuleRowId = rule.RowId // FK to Rule.RuleID (numeric)
		} else {
			// Fallback: check master in case rule was defined in another SARIF file
			masterResult.rulesMu.RLock()
			if masterRule, found := masterResult.Rules[ruleIDStr]; found {
				pbAlert.RuleRowId = masterRule.RowId
			} else {
				ts.logger.Warn("rule not found for alert, RuleRowId will be 0",
					"rule_id", ruleIDStr)
			}
			masterResult.rulesMu.RUnlock()
		}

		// Extract location information
		ts.extractLocation(pbAlert, result)

		// Extract fingerprints
		if result.PartialFingerprint != nil {
			if primaryHash, ok := result.PartialFingerprint["primaryLocationLineHash"]; ok {
				pbAlert.ResultFingerprint = &primaryHash
			}
		}

		// Extract code flow information
		ts.extractCodeFlow(pbAlert, result)

		alerts = append(alerts, pbAlert)
	}

	return alerts
}

// extractLocation extracts location information from a result
func (ts *TransformService) extractLocation(pbAlert *models.Alert, result models.SarifResult) {
	if len(result.Locations) == 0 {
		return
	}

	loc := result.Locations[0]
	physLoc := loc.PhysicalLocation

	// Extract file path
	pbAlert.FilePath = physLoc.ArtifactLocation.URI

	// Extract region information
	if physLoc.Region.StartLine > 0 {
		startLine := int32(physLoc.Region.StartLine)
		pbAlert.StartLine = &startLine

		if physLoc.Region.StartColumn > 0 {
			startColumn := int32(physLoc.Region.StartColumn)
			pbAlert.StartColumn = &startColumn
		}

		// Use endLine if present, otherwise fall back to startLine (single-line issue)
		if physLoc.Region.EndLine > 0 {
			endLine := int32(physLoc.Region.EndLine)
			pbAlert.EndLine = &endLine
		} else {
			pbAlert.EndLine = &startLine
		}

		if physLoc.Region.EndColumn > 0 {
			endColumn := int32(physLoc.Region.EndColumn)
			pbAlert.EndColumn = &endColumn
		}
	}

	// Extract code snippet from context region
	if physLoc.ContextRegion.Snippet.Text != "" {
		contextText := physLoc.ContextRegion.Snippet.Text
		pbAlert.CodeSnippetContext = &contextText

		// Extract exact code using region coordinates
		if physLoc.Region.StartLine > 0 {
			exactCode := ts.extractCodeFromSnippetStructured(contextText, physLoc.Region, physLoc.ContextRegion)
			if exactCode != "" {
				pbAlert.CodeSnippet = &exactCode
			}
		}
	}
}

// extractCodeFlow extracts code flow information (source/sink, step count)
func (ts *TransformService) extractCodeFlow(pbAlert *models.Alert, result models.SarifResult) {
	if len(result.CodeFlows) == 0 {
		return
	}

	codeFlow := result.CodeFlows[0]
	if len(codeFlow.ThreadFlows) == 0 {
		return
	}

	threadFlow := codeFlow.ThreadFlows[0]
	locations := threadFlow.Locations

	stepCount := int32(len(locations))
	pbAlert.StepCount = &stepCount

	// Find source and sink code snippets
	for _, tfLoc := range locations {
		physLoc := tfLoc.Location.PhysicalLocation

		// Check if this is source or sink
		var role string
		if len(tfLoc.Taxa) > 0 && tfLoc.Taxa[0].Properties != nil {
			if dataflowRole, ok := tfLoc.Taxa[0].Properties["CodeQL/DataflowRole"].(string); ok {
				role = dataflowRole
			}
		}

		// Extract snippet if this is source or sink
		if role == "source" || role == "sink" {
			snippet := physLoc.ContextRegion.Snippet.Text

			if snippet != "" {
				switch role {
				case "source":
					pbAlert.CodeSnippetSource = &snippet
				case "sink":
					pbAlert.CodeSnippetSink = &snippet
				}
			}
		}
	}
}

// extractCodeFromSnippetStructured extracts the exact code from a snippet using region coordinates.
// Handles both single-line and multi-line regions.
func (ts *TransformService) extractCodeFromSnippetStructured(snippetText string, region models.SarifRegion, contextRegion models.SarifRegion) string {
	if region.StartLine == 0 || region.StartColumn == 0 || region.EndColumn == 0 {
		return ""
	}

	if contextRegion.StartLine == 0 {
		return ""
	}

	lines := strings.Split(snippetText, "\n")
	startLineIndex := region.StartLine - contextRegion.StartLine

	if startLineIndex < 0 || startLineIndex >= len(lines) {
		return ""
	}

	// Determine the end line index (defaults to start line for single-line regions)
	endLine := region.EndLine
	if endLine == 0 {
		endLine = region.StartLine
	}
	endLineIndex := endLine - contextRegion.StartLine

	if endLineIndex < 0 || endLineIndex >= len(lines) {
		return ""
	}

	// Single-line region: extract substring from one line
	if startLineIndex == endLineIndex {
		line := lines[startLineIndex]
		start := region.StartColumn - 1
		end := region.EndColumn - 1

		if start < 0 || end > len(line) || start >= end {
			return ""
		}
		return line[start:end]
	}

	// Multi-line region: extract from start column on first line through end column on last line
	var sb strings.Builder

	// First line: from StartColumn to end of line
	firstLine := lines[startLineIndex]
	start := region.StartColumn - 1
	if start < 0 || start > len(firstLine) {
		return ""
	}
	sb.WriteString(firstLine[start:])

	// Middle lines: include entirely
	for i := startLineIndex + 1; i < endLineIndex; i++ {
		sb.WriteString("\n")
		sb.WriteString(lines[i])
	}

	// Last line: from start of line to EndColumn
	lastLine := lines[endLineIndex]
	end := region.EndColumn - 1
	if end < 0 || end > len(lastLine) {
		return ""
	}
	sb.WriteString("\n")
	sb.WriteString(lastLine[:end])

	return sb.String()
}
