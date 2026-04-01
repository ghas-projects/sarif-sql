package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ghas-projects/sarif-protobuf/internal/auth"
	"github.com/ghas-projects/sarif-protobuf/internal/github"
	"github.com/ghas-projects/sarif-protobuf/internal/models"
	"github.com/ghas-projects/sarif-protobuf/util"
)

// AnalysisService handles analysis operations
type AnalysisService struct {
	logger         *slog.Logger
	githubClient   *github.Client
	analysisId     string
	controllerRepo string
}

// NewAnalysisService creates a new AnalysisService instance
func NewAnalysisService(logger *slog.Logger, auth *auth.AuthConfig, analysisId string, controllerRepo string) *AnalysisService {
	return &AnalysisService{
		logger:         logger,
		githubClient:   github.NewClient(auth, logger),
		analysisId:     analysisId,
		controllerRepo: controllerRepo,
	}
}

// extractSarifFromZip extracts .sarif file from a zip archive
func extractSarifFromZip(zipPath string, outputDir string, repo string, controllerRepo string, logger *slog.Logger) (string, error) {
	// Open the zip file
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to open zip file %s: %w", zipPath, err)
	}
	defer reader.Close()

	// Look for .sarif file in the zip
	var sarifPath string
	for _, file := range reader.File {
		if strings.HasSuffix(strings.ToLower(file.Name), ".sarif") {
			// Construct output SARIF file path
			fullName := repo + "-" + controllerRepo
			sanitizedName := strings.ReplaceAll(fullName, "/", "-")
			sarifFilename := fmt.Sprintf("%s-results.sarif", sanitizedName)
			sarifPath = filepath.Join(outputDir, sarifFilename)

			// Open the file inside the zip
			rc, err := file.Open()
			if err != nil {
				return "", fmt.Errorf("failed to open file %s in zip: %w", file.Name, err)
			}

			outFile, err := os.Create(sarifPath)
			if err != nil {
				rc.Close()
				return "", fmt.Errorf("failed to create output file %s: %w", sarifPath, err)
			}

			// Copy the content
			written, err := io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()
			if err != nil {
				return "", fmt.Errorf("failed to extract SARIF file: %w", err)
			}

			logger.Info("SARIF file extracted from zip",
				"zip_path", zipPath,
				"sarif_path", sarifPath,
				"bytes_written", written)

			// Remove the zip file after successful extraction
			if err := os.Remove(zipPath); err != nil {
				logger.Warn("failed to remove zip file after extraction",
					"zip_path", zipPath,
					"error", err)
			} else {
				logger.Info("zip file removed after extraction",
					"zip_path", zipPath)
			}

			return sarifPath, nil
		}
	}

	return "", fmt.Errorf("no .sarif file found in zip archive %s", zipPath)
}

func processRepositoryDownload(ctx context.Context, workerId int, repoChan chan models.ScannedRepository, resultChan chan models.MRVAStatusResponse, outputDir string, analysisId string, controllerRepo string, githubClient *github.Client, logger *slog.Logger) {
	logger.Info("Worker started for artifact download", slog.Int("workerId", workerId))

	for scanned := range repoChan {
		if ctx.Err() != nil {
			logger.Info("Worker exiting due to context cancellation", slog.Int("workerId", workerId))
			return
		}

		repo := scanned.Repository.FullName

		// Skip repos that didn't succeed — no artifact to download
		if scanned.AnalysisStatus != "succeeded" {
			logger.Info("Skipping repository, analysis did not succeed",
				"repository", repo,
				"analysis_status", scanned.AnalysisStatus)
			resultChan <- models.MRVAStatusResponse{
				Repository:        scanned.Repository,
				AnalysisStatus:    scanned.AnalysisStatus,
				ArtifactSizeBytes: scanned.ArtifactSizeBytes,
				ResultCount:       scanned.ResultCount,
			}
			continue
		}

		// Fetch per-repo status to get artifact_url
		statusResponse, err := githubClient.FetchMRVAStatus(ctx, repo, analysisId, controllerRepo)
		if err != nil {
			logger.Error("Failed to fetch MRVA status",
				slog.String("repo", repo),
				slog.Any("error", err))
			resultChan <- models.MRVAStatusResponse{
				Repository:     scanned.Repository,
				AnalysisStatus: scanned.AnalysisStatus,
			}
			continue
		}

		if statusResponse.ArtifactURL == "" {
			logger.Warn("No artifact URL available for repository",
				"repository", repo)
			resultChan <- *statusResponse
			continue
		}

		logger.Info("MRVA analysis succeeded, downloading artifact",
			"repository", repo)

		artifactPath, err := githubClient.DownloadArtifactFile(ctx, repo, statusResponse.ArtifactURL, outputDir)
		if err != nil {
			logger.Error("Failed to download artifact",
				slog.String("repo", repo),
				slog.Any("error", err))
		} else {
			logger.Info("Artifact downloaded",
				"repository", repo,
				"path", artifactPath)

			sarifPath, err := extractSarifFromZip(artifactPath, outputDir, repo, controllerRepo, logger)
			if err != nil {
				logger.Error("Failed to extract SARIF file from artifact",
					slog.String("repo", repo),
					slog.String("artifact_path", artifactPath),
					slog.Any("error", err))
			} else {
				logger.Info("SARIF file extracted successfully",
					"repository", repo,
					"sarif_path", sarifPath)
				statusResponse.SarifFilePath = sarifPath
			}
		}

		logger.Info("Repository download processed",
			"repository", repo,
			"analysis_status", statusResponse.AnalysisStatus)
		resultChan <- *statusResponse
	}

	logger.Info("Worker completed artifact downloads", slog.Int("workerId", workerId))
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
		"controller_repo", s.controllerRepo)

	return nil
}

func (s *AnalysisService) DownloadAnalysisFiles(ctx context.Context, outputDir string) error {
	// Fetch repository list from the variant analysis summary API
	summary, err := s.githubClient.FetchMRVASummary(ctx, s.analysisId, s.controllerRepo)
	if err != nil {
		s.logger.Error("failed to fetch analysis summary",
			"analysis_id", s.analysisId,
			"controller_repo", s.controllerRepo,
			"error", err)
		return fmt.Errorf("failed to fetch analysis summary: %w", err)
	}

	var scannedRepositories []models.Repository
	for _, scanned := range summary.ScannedRepositories {
		scannedRepositories = append(scannedRepositories, scanned.Repository)
	}

	s.logger.Info("successfully scanned repositories fetched from analysis summary",
		"count", len(scannedRepositories))

	// Write analysis.json mapping summary data to the Analysis proto model
	if err := s.writeAnalysisJSON(summary, outputDir); err != nil {
		s.logger.Error("failed to write analysis.json", "error", err)
		return fmt.Errorf("failed to write analysis.json: %w", err)
	}

	// Write repos.json file containing all repositories in the analysis
	if err := s.writeReposJSON(summary, outputDir); err != nil {
		s.logger.Error("failed to write repos.json", "error", err)
		return fmt.Errorf("failed to write repos.json: %w", err)
	}

	// Calculate optimal number of workers for concurrent downloads
	workers := util.CalculateOptimalWorkers(len(scannedRepositories))

	s.logger.Info("starting sarif files download",
		"analysis_id", s.analysisId,
		"controller_repo", s.controllerRepo,
		"repository_count", len(scannedRepositories),
		"workers", workers)

	repoChan := make(chan models.ScannedRepository, len(scannedRepositories))
	resultsChan := make(chan models.MRVAStatusResponse, len(scannedRepositories))

	var wg sync.WaitGroup

	s.logger.Info("spawning worker goroutines",
		"worker_count", workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerId int) {
			defer wg.Done()
			processRepositoryDownload(ctx, workerId, repoChan, resultsChan, outputDir, s.analysisId, s.controllerRepo, s.githubClient, s.logger)
		}(i)
	}

	for _, scanned := range summary.ScannedRepositories {
		repoChan <- scanned
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

				if err := s.generateMRVAStatusReport(summary, results); err != nil {
					s.logger.Error("failed to generate status report", "error", err)
					return err
				}
				return nil
			}
			results = append(results, result)
		}
	}
}

// writeAnalysisJSON creates an analysis.json file from the summary response
func (s *AnalysisService) writeAnalysisJSON(summary *models.MRVASummaryResponse, outputDir string) error {
	analysis := summary.ToAnalysisRecord(s.analysisId, s.controllerRepo)

	data, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal analysis JSON: %w", err)
	}

	filePath := filepath.Join(outputDir, "analysis.json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write analysis.json: %w", err)
	}

	s.logger.Info("analysis.json written successfully", "path", filePath)
	return nil
}

// writeReposJSON creates a repos.json file from all repositories in the summary
func (s *AnalysisService) writeReposJSON(summary *models.MRVASummaryResponse, outputDir string) error {
	records := models.ToRepositoryRecords(*summary, s.analysisId)

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal repos JSON: %w", err)
	}

	filePath := filepath.Join(outputDir, "repos.json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write repos.json: %w", err)
	}

	s.logger.Info("repos.json written successfully", "path", filePath)
	return nil
}

func (s *AnalysisService) GetAnalysisSummary(ctx context.Context) error {
	s.logger.Info("fetching analysis summary",
		"analysis_id", s.analysisId,
		"controller_repo", s.controllerRepo)
	summary, err := s.githubClient.FetchMRVASummary(ctx, s.analysisId, s.controllerRepo)
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
