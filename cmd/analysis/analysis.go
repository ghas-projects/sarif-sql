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
	reposFile       string
	repos           string
	analysisService *service.AnalysisService
	appId           string
	privateKey      string
	token           string
	baseURL         string
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

		// Validate that either token OR (app-id + private-key) is provided, but not both
		hasToken := token != ""
		hasAppCreds := appId != "" || privateKey != ""

		if !hasToken && !hasAppCreds {
			return fmt.Errorf("authentication required: provide either --token OR both --app-id and --private-key")
		}

		if hasToken && hasAppCreds {
			return fmt.Errorf("conflicting authentication methods: provide either --token OR (--app-id and --private-key), not both")
		}

		// If using app credentials, both app-id and private-key must be provided
		if hasAppCreds {
			if appId == "" {
				return fmt.Errorf("--app-id is required when using GitHub App authentication")
			}
			if privateKey == "" {
				return fmt.Errorf("--private-key is required when using GitHub App authentication")
			}
		}

		// Set default base URL if not provided
		if baseURL == "" {
			baseURL = models.DefaultBaseURL
		}

		// Initialize and store authentication config
		auth.Auth = &auth.AuthConfig{
			Token:      token,
			AppID:      appId,
			PrivateKey: privateKey,
			BaseURL:    baseURL,
		}

		logger.Info("authentication configured",
			"has_token", token != "",
			"has_app_creds", appId != "",
			"base_url", baseURL)

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

		if baseURL == "" {
			baseURL = models.DefaultBaseURL
		}

		// Get analysisId and controllerRepo from parent's persistent flags
		analysisId, _ := cmd.Flags().GetString("analysis-id")
		controllerRepo, _ := cmd.Flags().GetString("controller-repo")

		// Initialize the analysis service with all required data
		analysisService = service.NewAnalysisService(logger, auth.Auth, analysisId, controllerRepo, repositories)

		return nil
	},
}

func init() {

	// GitHub App authentication flags
	AnalysisCmd.PersistentFlags().StringVar(&appId, "app-id", "", "GitHub App ID (required if not using --token)")
	AnalysisCmd.PersistentFlags().StringVar(&privateKey, "private-key", "", "GitHub App private key PEM content (required if not using --token)")

	// PAT authentication flag
	AnalysisCmd.PersistentFlags().StringVar(&token, "token", "", "GitHub Personal Access Token (required if not using GitHub App authentication)")

	AnalysisCmd.PersistentFlags().StringVar(&reposFile, "repos-file", "", "Path to a file containing a list of repositories to analyze")
	AnalysisCmd.PersistentFlags().StringVar(&repos, "repos", "", "Comma-separated list of repositories to analyze in owner/name format")
	AnalysisCmd.PersistentFlags().StringVar(&baseURL, "base-url", "", "GitHub API base URL")

	AnalysisCmd.AddCommand(AnalysisStartCmd)
	AnalysisCmd.AddCommand(AnalysisSummaryCmd)
	AnalysisCmd.AddCommand(AnalysisDownloadCmd)
}
