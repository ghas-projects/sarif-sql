package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghas-projects/sarif-avro/internal/models"
)

// generateMRVAStatusReport creates a markdown report from MRVA status results
func (s *AnalysisService) generateMRVAStatusReport(results []models.MRVAStatusResponse) error {
	// Group results by status
	statusGroups := make(map[string][]models.MRVAStatusResponse)
	for _, result := range results {
		statusGroups[result.AnalysisStatus] = append(statusGroups[result.AnalysisStatus], result)
	}

	// Build the markdown content
	var md strings.Builder

	// Header
	fmt.Fprintf(&md, "# MRVA Status Report\n\n")
	md.WriteString(fmt.Sprintf("**Analysis ID:** `%s`\n\n", s.analysisId))
	md.WriteString(fmt.Sprintf("**Controller Repository:** `%s`\n\n", s.controllerRepo))
	md.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().Format("2006-01-02 15:04:05 MST")))
	md.WriteString(fmt.Sprintf("**Total Repositories:** %d\n\n", len(results)))

	// Summary section
	md.WriteString("## Summary\n\n")
	md.WriteString("| Status | Count |\n")
	md.WriteString("|--------|-------|\n")

	for status, repos := range statusGroups {
		md.WriteString(fmt.Sprintf("| %s | %d |\n", status, len(repos)))
	}
	md.WriteString("\n")

	// Detailed results by status
	md.WriteString("## Detailed Results\n\n")

	// Define status order for better readability
	statusOrder := []string{"succeeded", "pending", "in_progress", "failed", "canceled"}

	for _, status := range statusOrder {
		repos, exists := statusGroups[status]
		if !exists || len(repos) == 0 {
			continue
		}

		md.WriteString(fmt.Sprintf("### %s (%d)\n\n", strings.Title(strings.ReplaceAll(status, "_", " ")), len(repos)))
		md.WriteString("| Repository | Result Count | Database Commit | Artifact Size | SARIF Path |\n")
		md.WriteString("|------------|--------------|-----------------|---------------|------------|\n")

		for _, repo := range repos {
			artifactSize := formatBytes(repo.ArtifactSizeBytes)
			commitSHA := repo.DatabaseCommitSHA
			if len(commitSHA) > 7 {
				commitSHA = commitSHA[:7]
			}

			sarifPath := repo.SarifFilePath
			if sarifPath == "" {
				sarifPath = "-"
			}

			md.WriteString(fmt.Sprintf("| `%s` | %d | `%s` | %s | `%s` |\n",
				repo.Repository.FullName,
				repo.ResultCount,
				commitSHA,
				artifactSize,
				sarifPath))
		}
		md.WriteString("\n")
	}

	// Add any statuses not in the predefined order
	for status, repos := range statusGroups {
		if !contains(statusOrder, status) {
			md.WriteString(fmt.Sprintf("### %s (%d)\n\n", strings.Title(strings.ReplaceAll(status, "_", " ")), len(repos)))
			md.WriteString("| Repository | Result Count | Database Commit | Artifact Size | SARIF Path |\n")
			md.WriteString("|------------|--------------|-----------------|---------------|------------|\n")

			for _, repo := range repos {
				artifactSize := formatBytes(repo.ArtifactSizeBytes)
				commitSHA := repo.DatabaseCommitSHA
				if len(commitSHA) > 7 {
					commitSHA = commitSHA[:7]
				}

				sarifPath := repo.SarifFilePath
				if sarifPath == "" {
					sarifPath = "-"
				}

				md.WriteString(fmt.Sprintf("| `%s` | %d | `%s` | %s | `%s` |\n",
					repo.Repository.FullName,
					repo.ResultCount,
					commitSHA,
					artifactSize,
					sarifPath))
			}
			md.WriteString("\n")
		}
	}

	// Save the report
	sanitizedRepo := strings.ReplaceAll(s.controllerRepo, "/", "-")
	reportFileName := fmt.Sprintf("%s-%s-status-report.md", s.analysisId, sanitizedRepo)
	reportsDir := filepath.Join(".", "reports")

	// Create reports directory if it doesn't exist
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		return fmt.Errorf("failed to create reports directory: %w", err)
	}

	reportPath := filepath.Join(reportsDir, reportFileName)

	if err := os.WriteFile(reportPath, []byte(md.String()), 0644); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	s.logger.Info("MRVA status report generated",
		"path", reportPath,
		"total_repos", len(results))

	return nil
}

// formatBytes converts bytes to human-readable format
func formatBytes(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}

	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	sizes := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), sizes[exp])
}

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// generateMRVASummaryReport creates a markdown report from MRVA summary
func (s *AnalysisService) generateMRVASummaryReport(summary *models.MRVASummaryResponse) error {
	// Build the markdown content
	var md strings.Builder

	// Header
	md.WriteString("# MRVA Analysis Summary\n\n")
	md.WriteString(fmt.Sprintf("**Analysis ID:** `%s`\n\n", s.analysisId))
	md.WriteString(fmt.Sprintf("**Controller Repository:** `%s`\n\n", summary.ControllerRepo.FullName))
	md.WriteString(fmt.Sprintf("**Query Language:** `%s`\n\n", summary.QueryLanguage))
	md.WriteString(fmt.Sprintf("**Query Pack URL:** `%s`\n\n", summary.QueryPackURL))
	md.WriteString(fmt.Sprintf("**Created At:** %s\n\n", summary.CreatedAt))
	md.WriteString(fmt.Sprintf("**Completed At:** %s\n\n", summary.CompletedAt))
	md.WriteString(fmt.Sprintf("**Actions Workflow Run ID:** %d\n\n", summary.ActionsWorkflowRunID))
	md.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().Format("2006-01-02 15:04:05 MST")))

	// Overview Section
	md.WriteString("## Overview\n\n")
	totalScanned := len(summary.ScannedRepositories)
	totalSkipped := summary.SkippedRepositories.AccessMismatchRepositories.RepositoryCount
	totalNotFound := summary.NotFoundRepositories.RepositoryCount
	totalNoCodeQL := summary.NoCodeQLDBRepositories.RepositoryCount
	totalOverLimit := summary.OverLimitRepositories.RepositoryCount
	totalRepos := totalScanned + totalSkipped + totalNotFound + totalNoCodeQL + totalOverLimit

	md.WriteString("| Category | Count |\n")
	md.WriteString("|----------|-------|\n")
	md.WriteString(fmt.Sprintf("| Total Repositories | %d |\n", totalRepos))
	md.WriteString(fmt.Sprintf("| Successfully Scanned | %d |\n", totalScanned))
	md.WriteString(fmt.Sprintf("| Access Mismatch (Skipped) | %d |\n", totalSkipped))
	md.WriteString(fmt.Sprintf("| Not Found | %d |\n", totalNotFound))
	md.WriteString(fmt.Sprintf("| No CodeQL Database | %d |\n", totalNoCodeQL))
	md.WriteString(fmt.Sprintf("| Over Limit | %d |\n", totalOverLimit))
	md.WriteString("\n")

	// Scanned Repositories
	if totalScanned > 0 {
		md.WriteString("## Scanned Repositories\n\n")
		md.WriteString("| Repository | Analysis Status | Result Count | Artifact Size |\n")
		md.WriteString("|------------|-----------------|--------------|---------------|\n")
		for _, scannedRepo := range summary.ScannedRepositories {
			md.WriteString(fmt.Sprintf("| `%s` | %s | %d | %s |\n",
				scannedRepo.Repository.FullName,
				scannedRepo.AnalysisStatus,
				scannedRepo.ResultCount,
				formatBytes(scannedRepo.ArtifactSizeBytes)))
		}
		md.WriteString("\n")
	}

	// Skipped Repositories (Access Mismatch)
	if totalSkipped > 0 {
		md.WriteString("## Skipped Repositories (Access Mismatch)\n\n")
		md.WriteString("| Repository |\n")
		md.WriteString("|------------|\n")
		for _, repo := range summary.SkippedRepositories.AccessMismatchRepositories.Repositories {
			md.WriteString(fmt.Sprintf("| `%s` |\n", repo.FullName))
		}
		md.WriteString("\n")
	}

	// Not Found Repositories
	if totalNotFound > 0 {
		md.WriteString("## Not Found Repositories\n\n")
		md.WriteString("| Repository |\n")
		md.WriteString("|------------|\n")
		for _, repoName := range summary.NotFoundRepositories.Repositories {
			md.WriteString(fmt.Sprintf("| `%s` |\n", repoName))
		}
		md.WriteString("\n")
	}

	// No CodeQL Database Repositories
	if totalNoCodeQL > 0 {
		md.WriteString("## No CodeQL Database Repositories\n\n")
		md.WriteString("| Repository |\n")
		md.WriteString("|------------|\n")
		for _, repo := range summary.NoCodeQLDBRepositories.Repositories {
			md.WriteString(fmt.Sprintf("| `%s` |\n", repo.FullName))
		}
		md.WriteString("\n")
	}

	// Over Limit Repositories
	if totalOverLimit > 0 {
		md.WriteString("## Over Limit Repositories\n\n")
		md.WriteString("| Repository |\n")
		md.WriteString("|------------|\n")
		for _, repo := range summary.OverLimitRepositories.Repositories {
			md.WriteString(fmt.Sprintf("| `%s` |\n", repo.FullName))
		}
		md.WriteString("\n")
	}

	// Save the report
	sanitizedRepo := strings.ReplaceAll(s.controllerRepo, "/", "-")
	reportFileName := fmt.Sprintf("%s-%s-summary-report.md", s.analysisId, sanitizedRepo)
	reportsDir := filepath.Join(".", "reports", "summary")

	// Create reports directory if it doesn't exist
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		return fmt.Errorf("failed to create reports summary directory: %w", err)
	}

	reportPath := filepath.Join(reportsDir, reportFileName)

	if err := os.WriteFile(reportPath, []byte(md.String()), 0644); err != nil {
		return fmt.Errorf("failed to write summary report: %w", err)
	}

	s.logger.Info("MRVA summary report generated",
		"path", reportPath,
		"total_repos", totalRepos)

	return nil
}
