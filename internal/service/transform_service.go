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
	"time"

	"github.com/ghas-projects/sarif-avro/internal/models"
	"github.com/ghas-projects/sarif-avro/util"
	"github.com/hamba/avro/ocf"
)

// AvroResultCollector provides thread-safe access to the master AvroResult structure
type AvroResultCollector struct {
	alertsMu sync.Mutex
	Alerts   []models.AvroAlert

	runOnce sync.Once // Ensures run is only set once
	Runs    models.AvroRun

	reposMu      sync.RWMutex
	Repositories map[string]models.AvroRepository

	rulesMu sync.RWMutex
	Rules   map[string]models.AvroRule

	// Atomic counters for unique ID generation across goroutines
	alertCounter      int64
	ruleCounter       int64
	repositoryCounter int64
}

// NewAvroResultCollector creates a new AvroResultCollector instance
func NewAvroResultCollector() *AvroResultCollector {
	return &AvroResultCollector{
		Repositories: make(map[string]models.AvroRepository),
		Rules:        make(map[string]models.AvroRule),
		Alerts:       make([]models.AvroAlert, 0),
		Runs:         models.AvroRun{},
	}
}

// AddAlerts adds multiple alerts in a single lock operation (more efficient)
func (s *AvroResultCollector) AddAlerts(alerts []models.AvroAlert) {
	s.alertsMu.Lock()
	s.Alerts = append(s.Alerts, alerts...)
	s.alertsMu.Unlock()
}

// AddRun adds a run to the master structure (only once, first goroutine wins)
func (s *AvroResultCollector) AddRun(run models.AvroRun) {
	s.runOnce.Do(func() {
		s.Runs = run
	})
}

// AddRepository adds or updates a repository (auto-dedups by key, skips if already exists)
func (s *AvroResultCollector) AddRepository(key string, repo models.AvroRepository) {
	s.reposMu.Lock()
	// Only add if not already present (optimization to avoid redundant overwrites)
	if _, exists := s.Repositories[key]; !exists {
		s.Repositories[key] = repo
	}
	s.reposMu.Unlock()
}

// AddRules adds multiple rules in a single lock operation (skips if already exists)
func (s *AvroResultCollector) AddRules(rules map[string]models.AvroRule) {
	s.rulesMu.Lock()
	for id, rule := range rules {
		// Only add if not already present (optimization to avoid redundant overwrites)
		if _, exists := s.Rules[id]; !exists {
			s.Rules[id] = rule
		}
	}
	s.rulesMu.Unlock()
}

// TransformService handles SARIF to Avro transformation
type TransformService struct {
	logger         *slog.Logger
	sarifDirPath   string
	outputDir      string
	analysisID     string
	controllerRepo string
}

func (ts *TransformService) WriteAvroFiles(avroResult *AvroResultCollector) error {

	// Ensure output directory exists
	if err := os.MkdirAll(ts.outputDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write runs to Avro file
	runsPath := filepath.Join(ts.outputDir, "run.avro")
	if err := ts.writeAvroFile(runsPath, "schema/run.avsc", []models.AvroRun{avroResult.Runs}); err != nil {
		return fmt.Errorf("failed to write runs Avro file: %w", err)
	}

	// Write repositories to Avro file
	reposPath := filepath.Join(ts.outputDir, "repository.avro")
	if err := ts.writeAvroFile(reposPath, "schema/repository.avsc", avroResult.Repositories); err != nil {
		return fmt.Errorf("failed to write repositories Avro file: %w", err)
	}

	// Write rules to Avro file
	rulesPath := filepath.Join(ts.outputDir, "rule.avro")
	if err := ts.writeAvroFile(rulesPath, "schema/rule.avsc", avroResult.Rules); err != nil {
		return fmt.Errorf("failed to write rules Avro file: %w", err)
	}

	// Write alerts to Avro file
	alertsPath := filepath.Join(ts.outputDir, "alert.avro")
	if err := ts.writeAvroFile(alertsPath, "schema/alert.avsc", avroResult.Alerts); err != nil {
		return fmt.Errorf("failed to write alerts Avro file: %w", err)
	}

	return nil

}

// writeAvroFile writes a slice of records to an Avro OCF file using linkedin/goavro
func (ts *TransformService) writeAvroFile(filePath string, schemaPath string, records interface{}) error {
	// Read schema file
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema file %s: %w", schemaPath, err)
	}
	schema := string(schemaBytes)

	// Create output file
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filePath, err)
	}
	defer file.Close()

	// Create Avro Encoder
	enc, err := ocf.NewEncoder(schema, file)
	if err != nil {
		return fmt.Errorf("failed to create Avro encoder: %w", err)
	}

	switch v := records.(type) {
	case []models.AvroRun:
		for _, run := range v {
			if err := enc.Encode(run); err != nil {
				return fmt.Errorf("failed to encode run record: %w", err)
			}
		}
		ts.logger.Info("successfully wrote runs to Avro file",
			"file", filePath)
	case map[string]models.AvroRepository:
		for _, repo := range v {
			if err := enc.Encode(repo); err != nil {
				return fmt.Errorf("failed to encode repository record: %w", err)
			}
		}
		ts.logger.Info("successfully wrote repositories to Avro file",
			"file", filePath)
	case map[string]models.AvroRule:
		for _, rule := range v {
			if err := enc.Encode(rule); err != nil {
				return fmt.Errorf("failed to encode rule record: %w", err)
			}
		}
		ts.logger.Info("successfully wrote rules to Avro file",
			"file", filePath)
	case []models.AvroAlert:
		for _, alert := range v {
			if err := enc.Encode(alert); err != nil {
				return fmt.Errorf("failed to encode alert record: %w", err)
			}
		}
		ts.logger.Info("successfully wrote alerts to Avro file",
			"file", filePath)
	default:
		return fmt.Errorf("unsupported record type for Avro encoding")
	}

	if err := enc.Flush(); err != nil {
		return fmt.Errorf("failed to flush encoder: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
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

// Transform converts SARIF files to Avro format using Option B (direct write with mutex)
func (ts *TransformService) Transform(ctx context.Context) (avroResult *AvroResultCollector, err error) {
	// Get number of files in directory
	sarifFiles, err := os.ReadDir(ts.sarifDirPath)

	if err != nil {
		return nil, fmt.Errorf("failed to read SARIF directory: %w", err)
	}

	// Calculate optimal number of workers for concurrent processing
	workers := util.CalculateOptimalWorkers(len(sarifFiles))

	ts.logger.Info("starting SARIF files processing",
		"analysis_id", ts.analysisID,
		"controller_repo", ts.controllerRepo,
		"sarif_file_count", len(sarifFiles),
		"workers", workers)

	// Create the master structure that all workers will write to directly
	masterResult := NewAvroResultCollector()
	sarifChan := make(chan string, len(sarifFiles))

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

	// Send all SARIF files to the channel
	for _, sarifFile := range sarifFiles {
		sarifChan <- sarifFile.Name()
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
func (ts *TransformService) processSarifFiles(ctx context.Context, workerId int, sarifChan <-chan string, masterResult *AvroResultCollector) {
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

			// Extract and write run information directly to master
			avroRun := ts.extractRun(run, runIdx)
			masterResult.AddRun(avroRun)

			// Extract rules and write directly to master (with auto-deduplication)
			ruleMap := ts.extractRules(run, masterResult)
			masterResult.AddRules(ruleMap)

			// Extract repository info and write directly to master
			repoKey, repo := ts.extractRepository(run, sarifFile, masterResult)
			masterResult.AddRepository(repoKey, repo)

			// Extract alerts and write directly to master (pass repo ID, run ID, and ruleMap for FK references)
			alerts := ts.extractAlerts(run, avroRun.RunID, repo.RepositoryID, ruleMap, masterResult)
			masterResult.AddAlerts(alerts)

			ts.logger.Info("transformed SARIF run to Avro",
				"worker_id", workerId,
				"sarif_file", sarifFile,
				"run_index", runIdx,
				"alerts_count", len(alerts),
				"rules_count", len(ruleMap))
		}
	}

}

// extractRun extracts run information from SARIF
func (ts *TransformService) extractRun(run models.SarifRun, runIdx int) models.AvroRun {
	avroRun := models.AvroRun{
		RunID:      runIdx,
		AnalysisID: ts.analysisID,
	}

	controllerRepo := ts.controllerRepo
	avroRun.ControllerRepo = &controllerRepo
	date := time.Now().Format("2006-01-02 15:04:05")
	avroRun.Date = &date

	// Extract tool information
	avroRun.ToolName = run.Tool.Driver.Name
	if run.Tool.Driver.SemanticVersion != "" {
		version := run.Tool.Driver.SemanticVersion
		avroRun.ToolVersion = &version
	}

	return avroRun
}

// extractRules extracts rules from SARIF run
func (ts *TransformService) extractRules(run models.SarifRun, masterResult *AvroResultCollector) map[string]models.AvroRule {
	ruleMap := make(map[string]models.AvroRule)

	for _, rule := range run.Tool.Driver.Rules {
		var avroRule models.AvroRule

		// Extract rule string ID
		ruleStringID := rule.ID
		avroRule.ID = rule.ID

		// Check if rule already exists in master (to reuse ID across goroutines)
		masterResult.rulesMu.RLock()
		existingRule, exists := masterResult.Rules[ruleStringID]
		masterResult.rulesMu.RUnlock()

		if exists {
			// Reuse existing numeric ID for consistency
			avroRule.RuleID = existingRule.RuleID
		} else {
			// Generate new unique rule ID using atomic counter (1-based for primary key)
			avroRule.RuleID = int(atomic.AddInt64(&masterResult.ruleCounter, 1))
		}

		avroRule.RuleName = rule.Name

		// Extract properties if present
		if rule.Properties != nil {
			if desc, ok := rule.Properties["description"].(string); ok {
				avroRule.RuleDescription = &desc
			}
			if kind, ok := rule.Properties["kind"].(string); ok {
				avroRule.Kind = kind
			}
			if tags, ok := rule.Properties["tags"].([]interface{}); ok {
				var tagStrings []string
				for _, tag := range tags {
					if tagStr, ok := tag.(string); ok {
						tagStrings = append(tagStrings, tagStr)
					}
				}
				avroRule.PropertyTags = tagStrings
			}
			if severity, ok := rule.Properties["problem.severity"].(string); ok {
				avroRule.SeverityLevel = &severity
			}
		}

		ruleMap[avroRule.ID] = avroRule
	}

	return ruleMap
}

// extractAlerts extracts alerts from SARIF run
func (ts *TransformService) extractAlerts(run models.SarifRun, runID int, repositoryID int, ruleMap map[string]models.AvroRule, masterResult *AvroResultCollector) []models.AvroAlert {
	var alerts []models.AvroAlert

	for _, result := range run.Results {
		// Generate unique alert ID using atomic counter (1-based for primary key)
		alertID := int(atomic.AddInt64(&masterResult.alertCounter, 1))

		avroAlert := models.AvroAlert{
			AlertID:      alertID,
			AnalysisID:   runID,        // FK to Run.RunID
			RepositoryID: repositoryID, // FK to Repository.RepositoryID
			Message:      result.Message.Text,
		}

		// Get Rule ID (convert string identifier to numeric PK)
		ruleIDStr := result.RuleID
		// First check local ruleMap
		if rule, found := ruleMap[ruleIDStr]; found {
			avroAlert.RuleID = rule.RuleID // FK to Rule.RuleID (numeric)
		} else {
			// Fallback: check master in case rule was defined in another SARIF file
			masterResult.rulesMu.RLock()
			if masterRule, found := masterResult.Rules[ruleIDStr]; found {
				avroAlert.RuleID = masterRule.RuleID
			}
			masterResult.rulesMu.RUnlock()
		}

		// Extract location information
		ts.extractLocation(&avroAlert, result)

		// Extract fingerprints
		if result.PartialFingerprint != nil {
			if primaryHash, ok := result.PartialFingerprint["primaryLocationLineHash"]; ok {
				avroAlert.ResultFingerprint = &primaryHash
			}
		}

		// Extract code flow information
		ts.extractCodeFlow(&avroAlert, result)

		alerts = append(alerts, avroAlert)
	}

	return alerts
}

// extractLocation extracts location information from a result
func (ts *TransformService) extractLocation(avroAlert *models.AvroAlert, result models.SarifResult) {
	if len(result.Locations) == 0 {
		return
	}

	loc := result.Locations[0]
	physLoc := loc.PhysicalLocation

	// Extract file path
	avroAlert.FilePath = physLoc.ArtifactLocation.URI

	// Extract region information
	if physLoc.Region.StartLine > 0 {
		startLine := physLoc.Region.StartLine
		avroAlert.StartLine = &startLine

		if physLoc.Region.StartColumn > 0 {
			startColumn := physLoc.Region.StartColumn
			avroAlert.StartColumn = &startColumn
		}

		// Use endLine if present, otherwise fall back to startLine (single-line issue)
		if physLoc.Region.EndLine > 0 {
			endLine := physLoc.Region.EndLine
			avroAlert.EndLine = &endLine
		} else {
			avroAlert.EndLine = &startLine
		}

		if physLoc.Region.EndColumn > 0 {
			endColumn := physLoc.Region.EndColumn
			avroAlert.EndColumn = &endColumn
		}
	}

	// Extract code snippet from context region
	if physLoc.ContextRegion.Snippet.Text != "" {
		contextText := physLoc.ContextRegion.Snippet.Text
		avroAlert.CodeSnippetContext = &contextText

		// Extract exact code using region coordinates
		if physLoc.Region.StartLine > 0 {
			exactCode := ts.extractCodeFromSnippetStructured(contextText, physLoc.Region, physLoc.ContextRegion)
			if exactCode != "" {
				avroAlert.CodeSnippet = &exactCode
			}
		}
	}
}

// extractCodeFlow extracts code flow information (source/sink, step count)
func (ts *TransformService) extractCodeFlow(avroAlert *models.AvroAlert, result models.SarifResult) {
	if len(result.CodeFlows) == 0 {
		return
	}

	codeFlow := result.CodeFlows[0]
	if len(codeFlow.ThreadFlows) == 0 {
		return
	}

	threadFlow := codeFlow.ThreadFlows[0]
	locations := threadFlow.Locations

	stepCount := len(locations)
	avroAlert.StepCount = &stepCount

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
					avroAlert.CodeSnippetSource = &snippet
				case "sink":
					avroAlert.CodeSnippetSink = &snippet
				}
			}
		}
	}
}

// extractCodeFromSnippetStructured extracts the exact code from a snippet using region coordinates
func (ts *TransformService) extractCodeFromSnippetStructured(snippetText string, region models.SarifRegion, contextRegion models.SarifRegion) string {
	if region.StartLine == 0 || region.StartColumn == 0 || region.EndColumn == 0 {
		return ""
	}

	if contextRegion.StartLine == 0 {
		return ""
	}

	lines := strings.Split(snippetText, "\n")
	lineIndex := region.StartLine - contextRegion.StartLine

	if lineIndex < 0 || lineIndex >= len(lines) {
		return ""
	}

	line := lines[lineIndex]
	start := region.StartColumn - 1
	end := region.EndColumn - 1

	if start < 0 || end > len(line) || start >= end {
		return ""
	}

	return line[start:end]
}

// extractRepository creates repository information from SARIF file
func (ts *TransformService) extractRepository(run models.SarifRun, sarifFile string, masterResult *AvroResultCollector) (string, models.AvroRepository) {

	repo := models.AvroRepository{
		RepositoryName: sarifFile, // Fallback to filename
		RepositoryURL:  "",
	}

	// Extract from versionControlProvenance if available
	if len(run.VersionControlProvenance) > 0 {
		repoUri := run.VersionControlProvenance[0].RepositoryURI
		repo.RepositoryURL = repoUri

		// Extract repository name from URL (e.g., "mrva-security-demo/anaconda")
		if parts := strings.Split(repoUri, "/"); len(parts) >= 2 {
			repo.RepositoryName = parts[len(parts)-2] + "/" + parts[len(parts)-1]
		}
	}

	// Use repository URL as the unique identifier
	repoKey := repo.RepositoryURL
	if repoKey == "" {
		repoKey = repo.RepositoryName // Fallback to name if no URL
	}

	// Check if repository already exists in master (to reuse ID across goroutines)
	masterResult.reposMu.RLock()
	existingRepo, exists := masterResult.Repositories[repoKey]
	masterResult.reposMu.RUnlock()

	if exists {
		// Reuse existing repository ID
		repo.RepositoryID = existingRepo.RepositoryID
	} else {
		// Generate new unique repository ID using atomic counter (1-based for primary key)
		repo.RepositoryID = int(atomic.AddInt64(&masterResult.repositoryCounter, 1))
	}

	return repoKey, repo
}
