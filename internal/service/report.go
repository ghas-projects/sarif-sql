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
		md.WriteString("| Repository | Result Count | Database Commit | Artifact Size |\n")
		md.WriteString("|------------|--------------|-----------------|---------------|\n")

		for _, repo := range repos {
			artifactSize := formatBytes(repo.ArtifactSizeBytes)
			commitSHA := repo.DatabaseCommitSHA
			if len(commitSHA) > 7 {
				commitSHA = commitSHA[:7]
			}

			md.WriteString(fmt.Sprintf("| `%s` | %d | `%s` | %s |\n",
				repo.Repository.FullName,
				repo.ResultCount,
				commitSHA,
				artifactSize))
		}
		md.WriteString("\n")
	}

	// Add any statuses not in the predefined order
	for status, repos := range statusGroups {
		if !contains(statusOrder, status) {
			md.WriteString(fmt.Sprintf("### %s (%d)\n\n", strings.Title(strings.ReplaceAll(status, "_", " ")), len(repos)))
			md.WriteString("| Repository | Result Count | Database Commit | Artifact Size |\n")
			md.WriteString("|------------|--------------|-----------------|---------------|\n")

			for _, repo := range repos {
				artifactSize := formatBytes(repo.ArtifactSizeBytes)
				commitSHA := repo.DatabaseCommitSHA
				if len(commitSHA) > 7 {
					commitSHA = commitSHA[:7]
				}

				md.WriteString(fmt.Sprintf("| `%s` | %d | `%s` | %s |\n",
					repo.Repository.FullName,
					repo.ResultCount,
					commitSHA,
					artifactSize))
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
