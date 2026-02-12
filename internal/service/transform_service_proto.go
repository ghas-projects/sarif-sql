package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ghas-projects/sarif-avro/internal/models"
	pb "github.com/ghas-projects/sarif-avro/proto/sarifpb"
	"github.com/ghas-projects/sarif-avro/util"
	"google.golang.org/protobuf/proto"
)

// ResultCollector provides thread-safe access to the master result structure
type ResultCollector struct {
	alertsMu sync.Mutex
	Alerts   []*pb.Alert

	runOnce sync.Once // Ensures run is only set once
	Runs    *pb.Run

	reposMu      sync.RWMutex
	Repositories map[string]*pb.Repository

	rulesMu sync.RWMutex
	Rules   map[string]*pb.Rule

	// Atomic counters for unique ID generation across goroutines
	alertCounter      int64
	ruleCounter       int64
	repositoryCounter int64
}

// NewResultCollector creates a new ResultCollector instance
func NewResultCollector() *ResultCollector {
	return &ResultCollector{
		Repositories: make(map[string]*pb.Repository),
		Rules:        make(map[string]*pb.Rule),
		Alerts:       make([]*pb.Alert, 0),
		Runs:         &pb.Run{},
	}
}

// AddAlerts adds multiple alerts in a single lock operation (more efficient)
func (s *ResultCollector) AddAlerts(alerts []*pb.Alert) {
	s.alertsMu.Lock()
	s.Alerts = append(s.Alerts, alerts...)
	s.alertsMu.Unlock()
}

// AddRun adds a run to the master structure (only once, first goroutine wins)
func (s *ResultCollector) AddRun(run *pb.Run) {
	s.runOnce.Do(func() {
		s.Runs = run
	})
}

// AddRepository adds or updates a repository (auto-dedups by key, skips if already exists)
func (s *ResultCollector) AddRepository(key string, repo *pb.Repository) {
	s.reposMu.Lock()
	// Only add if not already present (optimization to avoid redundant overwrites)
	if _, exists := s.Repositories[key]; !exists {
		s.Repositories[key] = repo
	}
	s.reposMu.Unlock()
}

// AddRules adds multiple rules in a single lock operation (skips if already exists)
func (s *ResultCollector) AddRules(rules map[string]*pb.Rule) {
	s.rulesMu.Lock()
	for id, rule := range rules {
		// Only add if not already present (optimization to avoid redundant overwrites)
		if _, exists := s.Rules[id]; !exists {
			s.Rules[id] = rule
		}
	}
	s.rulesMu.Unlock()
}

// TransformService handles SARIF to Protobuf transformation
type TransformService struct {
	logger         *slog.Logger
	sarifDirPath   string
	outputDir      string
	analysisID     string
	controllerRepo string
}

func (ts *TransformService) WriteProtoFiles(result *ResultCollector) error {

	// Ensure output directory exists
	if err := os.MkdirAll(ts.outputDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write runs to proto file
	runList := &pb.RunList{Runs: []*pb.Run{result.Runs}}
	runsPath := filepath.Join(ts.outputDir, "run.pb")
	if err := ts.writeProtoFile(runsPath, runList); err != nil {
		return fmt.Errorf("failed to write runs proto file: %w", err)
	}
	ts.logger.Info("successfully wrote runs to proto file", "file", runsPath)

	// Write repositories to proto file
	repos := make([]*pb.Repository, 0, len(result.Repositories))
	for _, repo := range result.Repositories {
		repos = append(repos, repo)
	}
	repoList := &pb.RepositoryList{Repositories: repos}
	reposPath := filepath.Join(ts.outputDir, "repository.pb")
	if err := ts.writeProtoFile(reposPath, repoList); err != nil {
		return fmt.Errorf("failed to write repositories proto file: %w", err)
	}
	ts.logger.Info("successfully wrote repositories to proto file", "file", reposPath)

	// Write rules to proto file
	rules := make([]*pb.Rule, 0, len(result.Rules))
	for _, rule := range result.Rules {
		rules = append(rules, rule)
	}
	ruleList := &pb.RuleList{Rules: rules}
	rulesPath := filepath.Join(ts.outputDir, "rule.pb")
	if err := ts.writeProtoFile(rulesPath, ruleList); err != nil {
		return fmt.Errorf("failed to write rules proto file: %w", err)
	}
	ts.logger.Info("successfully wrote rules to proto file", "file", rulesPath)

	// Write alerts to proto file
	alertList := &pb.AlertList{Alerts: result.Alerts}
	alertsPath := filepath.Join(ts.outputDir, "alert.pb")
	if err := ts.writeProtoFile(alertsPath, alertList); err != nil {
		return fmt.Errorf("failed to write alerts proto file: %w", err)
	}
	ts.logger.Info("successfully wrote alerts to proto file", "file", alertsPath)

	return nil
}

// writeProtoFile marshals a protobuf message and writes it to a file
func (ts *TransformService) writeProtoFile(filePath string, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal proto message: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
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

// Transform converts SARIF files to Protobuf format
func (ts *TransformService) Transform(ctx context.Context) (result *ResultCollector, err error) {
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
	masterResult := NewResultCollector()
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

			// Extract and write run information directly to master
			pbRun := ts.extractRun(run, runIdx)
			masterResult.AddRun(pbRun)

			// Extract rules and write directly to master (with auto-deduplication)
			ruleMap := ts.extractRules(run, masterResult)
			masterResult.AddRules(ruleMap)

			// Extract repository info and write directly to master
			repoKey, repo := ts.extractRepository(run, sarifFile, masterResult)
			masterResult.AddRepository(repoKey, repo)

			// Extract alerts and write directly to master (pass repo ID and ruleMap for FK references)
			alerts := ts.extractAlerts(run, repo.RepositoryId, ruleMap, masterResult)
			masterResult.AddAlerts(alerts)

			ts.logger.Info("transformed SARIF run to proto",
				"worker_id", workerId,
				"sarif_file", sarifFile,
				"run_index", runIdx,
				"alerts_count", len(alerts),
				"rules_count", len(ruleMap))
		}
	}

}

// extractRun extracts run information from SARIF
func (ts *TransformService) extractRun(run models.SarifRun, runIdx int) *pb.Run {
	pbRun := &pb.Run{
		RunId:      int32(runIdx + 1),
		AnalysisId: ts.analysisID,
	}

	controllerRepo := ts.controllerRepo
	pbRun.ControllerRepo = &controllerRepo
	date := time.Now().Format("2006-01-02 15:04:05")
	pbRun.Date = &date

	// Extract tool information
	pbRun.ToolName = run.Tool.Driver.Name
	if run.Tool.Driver.SemanticVersion != "" {
		version := run.Tool.Driver.SemanticVersion
		pbRun.ToolVersion = &version
	}

	return pbRun
}

// extractRules extracts rules from SARIF run
func (ts *TransformService) extractRules(run models.SarifRun, masterResult *ResultCollector) map[string]*pb.Rule {
	ruleMap := make(map[string]*pb.Rule)

	for _, rule := range run.Tool.Driver.Rules {
		pbRule := &pb.Rule{}

		// Extract rule string ID
		ruleStringID := rule.ID
		pbRule.Id = rule.ID

		// Atomically check-and-reserve rule ID under write lock to prevent race conditions
		masterResult.rulesMu.Lock()
		if existingRule, exists := masterResult.Rules[ruleStringID]; exists {
			// Reuse existing numeric ID for consistency
			pbRule.RuleId = existingRule.RuleId
		} else {
			// Generate new unique rule ID using atomic counter (1-based for primary key)
			pbRule.RuleId = int32(atomic.AddInt64(&masterResult.ruleCounter, 1))
			// Reserve this ID immediately so other goroutines see it
			masterResult.Rules[ruleStringID] = pbRule
		}
		masterResult.rulesMu.Unlock()

		pbRule.RuleName = rule.Name

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

		ruleMap[pbRule.Id] = pbRule
	}

	return ruleMap
}

// extractAlerts extracts alerts from SARIF run
func (ts *TransformService) extractAlerts(run models.SarifRun, repositoryID int32, ruleMap map[string]*pb.Rule, masterResult *ResultCollector) []*pb.Alert {
	var alerts []*pb.Alert

	// Parse analysis ID from command input
	analysisID, _ := strconv.Atoi(ts.analysisID)

	for _, result := range run.Results {
		// Generate unique alert ID using atomic counter (1-based for primary key)
		alertID := int32(atomic.AddInt64(&masterResult.alertCounter, 1))

		pbAlert := &pb.Alert{
			AlertId:      alertID,
			AnalysisId:   int32(analysisID), // FK to analysis from command input
			RepositoryId: repositoryID,      // FK to Repository.RepositoryID
			Message:      result.Message.Text,
		}

		// Get Rule ID (convert string identifier to numeric PK)
		ruleIDStr := result.RuleID
		// First check local ruleMap
		if rule, found := ruleMap[ruleIDStr]; found {
			pbAlert.RuleId = rule.RuleId // FK to Rule.RuleID (numeric)
		} else {
			// Fallback: check master in case rule was defined in another SARIF file
			masterResult.rulesMu.RLock()
			if masterRule, found := masterResult.Rules[ruleIDStr]; found {
				pbAlert.RuleId = masterRule.RuleId
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
func (ts *TransformService) extractLocation(pbAlert *pb.Alert, result models.SarifResult) {
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
func (ts *TransformService) extractCodeFlow(pbAlert *pb.Alert, result models.SarifResult) {
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
func (ts *TransformService) extractRepository(run models.SarifRun, sarifFile string, masterResult *ResultCollector) (string, *pb.Repository) {

	repo := &pb.Repository{
		RepositoryName: sarifFile, // Fallback to filename
		RepositoryUrl:  "",
	}

	// Extract from versionControlProvenance if available
	if len(run.VersionControlProvenance) > 0 {
		repoUri := run.VersionControlProvenance[0].RepositoryURI
		repo.RepositoryUrl = repoUri

		// Extract repository name from URL (e.g., "mrva-security-demo/anaconda")
		if parts := strings.Split(repoUri, "/"); len(parts) >= 2 {
			repo.RepositoryName = parts[len(parts)-2] + "/" + parts[len(parts)-1]
		}
	}

	// Use repository URL as the unique identifier
	repoKey := repo.RepositoryUrl
	if repoKey == "" {
		repoKey = repo.RepositoryName // Fallback to name if no URL
	}

	// Atomically check-and-reserve repository ID under write lock to prevent race conditions
	masterResult.reposMu.Lock()
	if existingRepo, exists := masterResult.Repositories[repoKey]; exists {
		// Reuse existing repository ID
		repo.RepositoryId = existingRepo.RepositoryId
	} else {
		// Generate new unique repository ID using atomic counter (1-based for primary key)
		repo.RepositoryId = int32(atomic.AddInt64(&masterResult.repositoryCounter, 1))
		// Reserve this ID immediately so other goroutines see it
		masterResult.Repositories[repoKey] = repo
	}
	masterResult.reposMu.Unlock()

	return repoKey, repo
}
