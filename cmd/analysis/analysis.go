package analysis

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/ghas-projects/sarif-protobuf/internal/auth"
	"github.com/ghas-projects/sarif-protobuf/internal/models"
	"github.com/ghas-projects/sarif-protobuf/internal/service"
	"github.com/spf13/cobra"
)

var (
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

		// Get analysisId and controllerRepo from parent's persistent flags
		analysisId, _ := cmd.Flags().GetString("analysis-id")
		controllerRepo, _ := cmd.Flags().GetString("controller-repo")

		// Initialize the analysis service
		analysisService = service.NewAnalysisService(logger, auth.Auth, analysisId, controllerRepo)

		return nil
	},
}

func init() {

	// GitHub App authentication flags
	AnalysisCmd.PersistentFlags().StringVar(&appId, "app-id", "", "GitHub App ID (required if not using --token)")
	AnalysisCmd.PersistentFlags().StringVar(&privateKey, "private-key", "", "GitHub App private key PEM content (required if not using --token)")

	// PAT authentication flag
	AnalysisCmd.PersistentFlags().StringVar(&token, "token", "", "GitHub Personal Access Token (required if not using GitHub App authentication)")

	AnalysisCmd.PersistentFlags().StringVar(&baseURL, "base-url", "", "GitHub API base URL")

	AnalysisCmd.AddCommand(AnalysisStartCmd)
	AnalysisCmd.AddCommand(AnalysisSummaryCmd)
	AnalysisCmd.AddCommand(AnalysisDownloadCmd)
}
