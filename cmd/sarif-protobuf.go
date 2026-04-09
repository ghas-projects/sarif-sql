package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/ghas-projects/sarif-sql/cmd/analysis"
	"github.com/ghas-projects/sarif-sql/cmd/transform"
	"github.com/ghas-projects/sarif-sql/internal/models"
	"github.com/ghas-projects/sarif-sql/util"
	"github.com/spf13/cobra"
)

var (
	analysisId     string
	controllerRepo string
)

var rootCmd = &cobra.Command{
	Use:   "sarif-sql",
	Short: "A tool to convert SARIF files to Protobuf format",
	Long:  `sarif-sql is a command-line tool that converts SARIF (Static Analysis Results Interchange Format) files into SQL format for easier data processing and analysis.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

		// Generate log file path automatically
		logFilePath := util.GenerateLogFileName("sarif-sql")

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

	rootCmd.PersistentFlags().StringVar(&analysisId, "analysis-id", "", "Analysis ID for the MRVA analysis")
	rootCmd.MarkPersistentFlagRequired("analysis-id")
	rootCmd.PersistentFlags().StringVar(&controllerRepo, "controller-repo", "", "Specify the controller repository in owner/name format")
	rootCmd.MarkPersistentFlagRequired("controller-repo")

}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
