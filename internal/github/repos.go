package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/ghas-projects/sarif-avro/internal/auth"
	"github.com/ghas-projects/sarif-avro/internal/models"
)

func CheckMRVAStatus(ctx context.Context, workerId int, repoChan chan string, resultChan chan models.MRVAStatusResponse, analysisId string, controllerRepo string, auth *auth.AuthConfig, logger *slog.Logger) (*models.MRVAStatusResponse, error) {
	logger.Info("Worker started for MRVA status check", slog.Int("workerId", workerId))

	for repo := range repoChan {
		select {
		case <-ctx.Done():
			logger.Info("Worker exiting due to context cancellation", slog.Int("workerId", workerId))
			return nil, ctx.Err()
		default:
			statusResponse, err := fetchMRVAStatus(ctx, repo, analysisId, controllerRepo, auth, logger)
			if err != nil {
				logger.Error("Failed to fetch MRVA status",
					slog.String("repo", repo),
					slog.Any("error", err))
				continue
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

// fetchMRVAStatus fetches the MRVA status for a specific repository
func fetchMRVAStatus(ctx context.Context, repo string, analysisId string, controllerRepo string, auth *auth.AuthConfig, logger *slog.Logger) (*models.MRVAStatusResponse, error) {
	rt := GetAuthenticatedTransport(ctx, auth, logger, repo)
	client := &http.Client{Transport: rt}

	logger.Info("Checking MRVA status",
		"repository", repo,
		"analysis_id", analysisId,
		"controller_repo", controllerRepo)

	baseURL := auth.BaseURL

	apiUrl := fmt.Sprintf("%s/repos/%s/code-scanning/codeql/variant-analyses/%s/repos/%s", baseURL, controllerRepo, analysisId, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiUrl, nil)
	if err != nil {
		logger.Error("Failed to create request", slog.Any("error", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to execute request", slog.Any("error", err))
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response body", slog.Any("error", err))
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error("Failed to get MRVA status",
			slog.Int("status_code", resp.StatusCode),
			slog.String("response", string(body)))
		return nil, fmt.Errorf("failed to get MRVA status with status %d: %s", resp.StatusCode, string(body))
	}

	mrvaStatusResponse := &models.MRVAStatusResponse{}
	if err := json.Unmarshal(body, mrvaStatusResponse); err != nil {
		logger.Error("Failed to unmarshal response", slog.Any("error", err))
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return mrvaStatusResponse, nil
}
