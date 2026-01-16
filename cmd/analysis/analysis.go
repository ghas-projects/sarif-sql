package analysis

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/ghas-projects/sarif-avro/internal/auth"
	"github.com/ghas-projects/sarif-avro/internal/models"
	"github.com/ghas-projects/sarif-avro/internal/parser"
	"github.com/ghas-projects/sarif-avro/internal/service"
	"github.com/spf13/cobra"
)

var (
	analysisId      string
	controllerRepo  string
	reposFile       string
	repos           string
	analysisService *service.AnalysisService
)

var AnalysisCmd = &cobra.Command{
	Use:   "analysis",
	Short: "Manage MRVA analysis lifecycle and SARIF artifacts",
	Long: `The analysis command manages the lifecycle of MRVA analyses.
It allows users to initialize analysis directories, query analysis
status, and download SARIF artifacts into a structured layout.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Call parent's PersistentPreRunE if it exists
		if cmd.HasParent() {
			parent := cmd.Parent()
			if parent.PersistentPreRunE != nil {
				if err := parent.PersistentPreRunE(parent, args); err != nil {
					return err
				}
			}
		}

		// Get logger from config package (initialized by root command)
		logger := models.Logger
		if logger == nil {
			// Fallback if somehow not initialized
			logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))
		}

		// Validate that either repos or repos-file is provided
		if repos == "" && reposFile == "" {
			logger.Error("Either repos or repos-file must be provided.")
			return fmt.Errorf("either repos or repos-file must be provided")
		}

		var repositories []models.Repository
		var err error

		if reposFile != "" {
			repositories, err = parser.ParseRepositoriesFromFile(reposFile)
			if err != nil {
				logger.Error("failed to parse repositories from file",
					"file", reposFile,
					"error", err)
				return fmt.Errorf("failed to parse repositories from file: %w", err)
			}
		} else if repos != "" {
			repositories, err = parser.ParseRepositoriesFromString(repos)
			if err != nil {
				logger.Error("failed to parse repositories from string",
					"error", err)
				return fmt.Errorf("failed to parse repositories from string: %w", err)
			}
		} else {
			return fmt.Errorf("either --repos-file or --repos must be provided")
		}

		logger.Info("parsed repositories",
			"count", len(repositories))

		// Initialize the analysis service with all required data
		analysisService = service.NewAnalysisService(logger, auth.Auth, analysisId, controllerRepo, repositories)

		return nil
	},
}

func init() {

	AnalysisCmd.PersistentFlags().StringVar(&analysisId, "analysis-id", "", "Specify the MRVA analysis ID")
	AnalysisCmd.MarkPersistentFlagRequired("analysis-id")
	AnalysisCmd.PersistentFlags().StringVar(&controllerRepo, "controller-repo", "", "Specify the controller repository in owner/name format")
	AnalysisCmd.MarkPersistentFlagRequired("controller-repo")
	AnalysisCmd.PersistentFlags().StringVar(&reposFile, "repos-file", "", "Path to a file containing a list of repositories to analyze")
	AnalysisCmd.PersistentFlags().StringVar(&repos, "repos", "", "Comma-separated list of repositories to analyze in owner/name format")

	AnalysisCmd.AddCommand(AnalysisStartCmd)
	AnalysisCmd.AddCommand(AnalysisStatusCmd)
	AnalysisCmd.AddCommand(AnalysisDownloadCmd)
}
