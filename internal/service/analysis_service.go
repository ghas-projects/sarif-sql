package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ghas-projects/sarif-avro/internal/auth"
	"github.com/ghas-projects/sarif-avro/internal/github"
	"github.com/ghas-projects/sarif-avro/internal/models"
	"github.com/ghas-projects/sarif-avro/util"
)

// AnalysisService handles analysis operations
type AnalysisService struct {
	logger         *slog.Logger
	auth           *auth.AuthConfig
	analysisId     string
	controllerRepo string
	repositories   []models.Repository
}

// NewAnalysisService creates a new AnalysisService instance
func NewAnalysisService(logger *slog.Logger, auth *auth.AuthConfig, analysisId string, controllerRepo string, repositories []models.Repository) *AnalysisService {
	return &AnalysisService{
		logger:         logger,
		auth:           auth,
		analysisId:     analysisId,
		controllerRepo: controllerRepo,
		repositories:   repositories,
	}
}

func processRepositoryAnalysis(ctx context.Context, workerId int, repoChan chan string, resultChan chan models.MRVAStatusResponse, outputDir string, analysisId string, controllerRepo string, auth *auth.AuthConfig, logger *slog.Logger) (*models.MRVAStatusResponse, error) {
	logger.Info("Worker started for MRVA status check", slog.Int("workerId", workerId))

	for repo := range repoChan {
		select {
		case <-ctx.Done():
			logger.Info("Worker exiting due to context cancellation", slog.Int("workerId", workerId))
			return nil, ctx.Err()
		default:
			statusResponse, err := github.FetchMRVAStatus(ctx, repo, analysisId, controllerRepo, auth, logger)
			if err != nil {
				logger.Error("Failed to fetch MRVA status",
					slog.String("repo", repo),
					slog.Any("error", err))
				continue
			}

			if statusResponse.AnalysisStatus == "succeeded" {
				logger.Info("MRVA analysis succeeded, SARIF artifact is ready for download",
					"repository", repo)
				// Download SARIF file
				sarifPath, err := github.DownloadArtifactFile(ctx, repo, statusResponse.ArtifactURL, outputDir, auth, logger)
				statusResponse.SarifFilePath = sarifPath
				if err != nil {
					logger.Error("Failed to download SARIF file",
						slog.String("repo", repo),
						slog.Any("error", err))
				} else {
					logger.Info("SARIF file downloaded",
						"repository", repo,
						"path", sarifPath)
				}
			}

			logger.Info("Successfully retrieved MRVA status",
				"repository", repo,
				"analysis_status", statusResponse.AnalysisStatus)
			resultChan <- *statusResponse
		}
		logger.Info("Worker completed MRVA status check", slog.Int("workerId", workerId))
	}

	return nil, nil
}

// createAnalysisDirectory creates a directory structure for the analysis
func (s *AnalysisService) createAnalysisDirectory(ctx context.Context) error {
	if s.analysisId == "" {
		return fmt.Errorf("analysis ID cannot be empty")
	}

	if s.controllerRepo == "" {
		return fmt.Errorf("controller repo cannot be empty")
	}

	// Replace "/" in controller repo to avoid creating nested directories
	sanitizedRepo := strings.ReplaceAll(s.controllerRepo, "/", "-")

	// Create the analysis directory path with format: analysisId-controllerRepo
	dirName := fmt.Sprintf("%s-%s", s.analysisId, sanitizedRepo)
	analysisDir := filepath.Join(".", "analyses", dirName, "")

	// Create the directory structure
	if err := os.MkdirAll(analysisDir, 0755); err != nil {
		s.logger.Error("failed to create analysis directory",
			"analysis_id", s.analysisId,
			"controller_repo", s.controllerRepo,
			"path", analysisDir,
			"error", err)
		return fmt.Errorf("failed to create analysis directory: %w", err)
	}

	s.logger.Info("analysis directory created successfully",
		"analysis_id", s.analysisId,
		"controller_repo", s.controllerRepo,
		"path", analysisDir)

	return nil
}

// StartAnalysis initializes an analysis for the repositories
func (s *AnalysisService) StartAnalysis(ctx context.Context) error {
	// Create the main analysis directory with format: analysisId-controllerRepo
	if err := s.createAnalysisDirectory(ctx); err != nil {
		return err
	}

	s.logger.Info("analysis started successfully",
		"analysis_id", s.analysisId,
		"controller_repo", s.controllerRepo,
		"repository_count", len(s.repositories))

	return nil
}

func (s *AnalysisService) DownloadAnalysiFiles(ctx context.Context, outputDir string) error {

	// Calculate optimal number of workers for concurrent status checks
	workers := util.CalculateOptimalWorkers(len(s.repositories))

	s.logger.Info("starting sarif files download",
		"analysis_id", s.analysisId,
		"controller_repo", s.controllerRepo,
		"repository_count", len(s.repositories),
		"workers", workers)

	repoChan := make(chan string, len(s.repositories))
	resultsChan := make(chan models.MRVAStatusResponse, len(s.repositories))

	var wg sync.WaitGroup

	s.logger.Info("spawning worker goroutines",
		"worker_count", workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerId int) {
			defer wg.Done()
			processRepositoryAnalysis(ctx, workerId, repoChan, resultsChan, outputDir, s.analysisId, s.controllerRepo, s.auth, s.logger)
		}(i)
	}

	for _, repo := range s.repositories {
		repoChan <- repo.FullName
	}
	close(repoChan)

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results from the channel
	var results []models.MRVAStatusResponse
	for {
		select {
		case <-ctx.Done():
			s.logger.Warn("status check cancelled",
				"error", ctx.Err(),
				"partial_results", len(results))
			return ctx.Err()
		case result, ok := <-resultsChan:
			if !ok {
				// Channel closed, all results collected - generate report and return
				s.logger.Info("status check completed",
					"total_results", len(results))

				if err := s.generateMRVAStatusReport(results); err != nil {
					s.logger.Error("failed to generate status report", "error", err)
					return err
				}
				return nil
			}
			results = append(results, result)
		}
	}
}

func (s *AnalysisService) GetAnalysisSummary(ctx context.Context) error {
	s.logger.Info("fetching analysis summary",
		"analysis_id", s.analysisId,
		"controller_repo", s.controllerRepo)
	summary, err := github.FetchMRVASummary(ctx, s.analysisId, s.controllerRepo, s.auth, s.logger)
	if err != nil {
		s.logger.Error("failed to fetch analysis summary",
			"analysis_id", s.analysisId,
			"controller_repo", s.controllerRepo,
			"error", err)
		return fmt.Errorf("failed to fetch analysis summary: %w", err)
	}

	// Create a report for summary
	if err := s.generateMRVASummaryReport(summary); err != nil {
		s.logger.Error("failed to generate summary report", "error", err)
		return fmt.Errorf("failed to generate summary report: %w", err)
	}

	s.logger.Info("analysis summary fetched successfully",
		"analysis_id", s.analysisId,
		"controller_repo", s.controllerRepo)

	return nil
}
