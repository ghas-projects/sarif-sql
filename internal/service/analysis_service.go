package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghas-projects/sarif-avro/internal/auth"
	"github.com/ghas-projects/sarif-avro/internal/models"
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
