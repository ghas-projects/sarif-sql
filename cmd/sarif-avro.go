package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/ghas-projects/sarif-avro/cmd/analysis"
	"github.com/ghas-projects/sarif-avro/cmd/transform"
	"github.com/ghas-projects/sarif-avro/internal/auth"
	"github.com/ghas-projects/sarif-avro/internal/models"
	"github.com/ghas-projects/sarif-avro/util"
	"github.com/spf13/cobra"
)

var (
	appId      string
	privateKey string
	token      string
	baseURL    string
)

var rootCmd = &cobra.Command{
	Use:   "sarif-avro",
	Short: "A tool to convert SARIF files to Avro format",
	Long:  `sarif-avro is a command-line tool that converts SARIF (Static Analysis Results Interchange Format) files into Avro format for easier data processing and analysis.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

		// Generate log file path automatically
		logFilePath := util.GenerateLogFileName("sarif-avro")

		// Initialize logger with automatic log file
		loggerConfig := util.LoggerConfig{
			LogFilePath: logFilePath,
			LogLevel:    slog.LevelInfo,
		}
		logger, closer, err := util.NewLogger(loggerConfig)
		if err != nil {
			return fmt.Errorf("failed to initialize logger: %w", err)
		}

		if closer != nil {
			cmd.SetContext(context.WithValue(cmd.Context(), "logCloser", closer))
		}
		logger.Info("Logging initialized", slog.String("log_file", logFilePath))

		models.Logger = logger

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

		return nil

	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if closer, ok := cmd.Context().Value("logCloser").(io.Closer); ok && closer != nil {
			return closer.Close()
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(analysis.AnalysisCmd)
	rootCmd.AddCommand(transform.TransformCmd)

	// GitHub App authentication flags
	rootCmd.PersistentFlags().StringVar(&appId, "app-id", "", "GitHub App ID (required if not using --token)")
	rootCmd.PersistentFlags().StringVar(&privateKey, "private-key", "", "GitHub App private key PEM content (required if not using --token)")

	// PAT authentication flag
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "GitHub Personal Access Token (required if not using GitHub App authentication)")

	// Common flags
	rootCmd.PersistentFlags().StringVar(&baseURL, "base-url", "", "GitHub API base URL")

	if baseURL == "" {
		baseURL = models.DefaultBaseURL
	}

}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
