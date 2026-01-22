package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghas-projects/sarif-avro/internal/auth"
	"github.com/ghas-projects/sarif-avro/internal/models"
)

// FetchMRVAStatus fetches the MRVA status for a specific repository
func FetchMRVAStatus(ctx context.Context, repo string, analysisId string, controllerRepo string, auth *auth.AuthConfig, logger *slog.Logger) (*models.MRVAStatusResponse, error) {
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

// DownloadArtifactFile downloads an artifact file from the artifact URL for a repository
// The file type is determined from the HTTP response headers (Content-Type or Content-Disposition)
func DownloadArtifactFile(ctx context.Context, repoFullName string, artifactURL string, outputDir string, auth *auth.AuthConfig, logger *slog.Logger) (string, error) {
	rt := GetAuthenticatedTransport(ctx, auth, logger, repoFullName)
	client := &http.Client{Transport: rt}

	logger.Info("downloading artifact file",
		"repository", repoFullName,
		"artifact_url", artifactURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download artifact file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to download artifact file with status %d: %s", resp.StatusCode, string(body))
	}

	// Generate filename from repository name
	// Replace / with - to avoid directory creation
	sanitizedRepo := strings.ReplaceAll(repoFullName, "/", "-")
	filename := filepath.Join(outputDir, fmt.Sprintf("%s%s", sanitizedRepo, ".zip"))

	// Get absolute path
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %s: %w", filename, err)
	}

	// Create the output file
	outFile, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("failed to create output file %s: %w", filename, err)
	}
	defer outFile.Close()

	// Copy the response body to the file
	written, err := io.Copy(outFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write artifact file: %w", err)
	}

	logger.Info("artifact file downloaded successfully",
		"repository", repoFullName,
		"filename", absPath,
		"file_type", ".zip",
		"bytes_written", written)

	return absPath, nil
}

// Fetch mrva summary for a specific analysis
func FetchMRVASummary(ctx context.Context, analysisId string, controllerRepo string, auth *auth.AuthConfig, logger *slog.Logger) (*models.MRVASummaryResponse, error) {
	rt := GetAuthenticatedTransport(ctx, auth, logger, controllerRepo)
	client := &http.Client{Transport: rt}

	logger.Info("Fetching MRVA summary",
		"analysis_id", analysisId,
		"controller_repo", controllerRepo)

	baseURL := auth.BaseURL

	apiUrl := fmt.Sprintf("%s/repos/%s/code-scanning/codeql/variant-analyses/%s", baseURL, controllerRepo, analysisId)
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
		logger.Error("Failed to get MRVA summary",
			slog.Int("status_code", resp.StatusCode),
			slog.String("response", string(body)))
		return nil, fmt.Errorf("failed to get MRVA summary with status %d: %s", resp.StatusCode, string(body))
	}

	mrvaSummaryResponse := &models.MRVASummaryResponse{}
	if err := json.Unmarshal(body, mrvaSummaryResponse); err != nil {
		logger.Error("Failed to unmarshal response", slog.Any("error", err))
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return mrvaSummaryResponse, nil
}
