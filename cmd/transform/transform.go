package transform

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/ghas-projects/sarif-protobuf/internal/models"
	"github.com/ghas-projects/sarif-protobuf/internal/service"
	"github.com/ghas-projects/sarif-protobuf/internal/store"
	"github.com/spf13/cobra"
)

var (
	sarifDir       string
	outputDir      string
	analysisID     string
	controllerRepo string
)

var TransformCmd = &cobra.Command{
	Use:   "transform",
	Short: "Transform SARIF files into Protobuf format",
	Long: `The transform command converts SARIF result files into Protobuf format
for reporting and analytics purposes.

Examples:
  # Transform a single SARIF file
  sarif-protobuf transform --sarif-file analysis.sarif --analysis-id 12345 --repo my-org/my-repo --output ./output

  # Transform all SARIF files in a directory
  sarif-protobuf transform --sarif-dir ./analyses/12345 --analysis-id 12345 --output ./output`,
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
		return nil
	},
	RunE: runTransform,
}

func init() {
	TransformCmd.Flags().StringVar(&sarifDir, "sarif-directory", "", "Path to directory containing SARIF files")
	TransformCmd.Flags().StringVar(&outputDir, "output", "./output", "Output directory for SQLite database")
	TransformCmd.Flags().StringVar(&analysisID, "analysis-id", "", "Analysis ID associated with the SARIF files")
	TransformCmd.Flags().StringVar(&controllerRepo, "controller-repo", "", "Controller repository associated with the analysis")

	TransformCmd.MarkFlagRequired("sarif-directory")
	TransformCmd.MarkFlagRequired("analysis-id")
	TransformCmd.MarkFlagRequired("controller-repo")
}

func runTransform(cmd *cobra.Command, args []string) error {
	logger := models.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	svc := service.NewTransformService(logger, sarifDir, outputDir, analysisID, controllerRepo)

	// Use command context which handles cancellation (Ctrl+C)
	result, err := svc.Transform(cmd.Context())
	if err != nil {
		return err
	}

	// Open SQLite store and write all results in a single transaction
	db, err := store.NewSQLiteStore(outputDir)
	if err != nil {
		return fmt.Errorf("open SQLite store: %w", err)
	}
	defer db.Close()

	tx, err := db.BeginTx()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := db.WriteAnalysis(tx, result.Analysis); err != nil {
		return fmt.Errorf("write analysis: %w", err)
	}

	repos := make([]*models.SQLRepository, 0, len(result.Repositories))
	for _, r := range result.Repositories {
		repos = append(repos, r)
	}
	if err := db.WriteRepositories(tx, repos); err != nil {
		return fmt.Errorf("write repositories: %w", err)
	}

	rules := make([]*models.Rule, 0, len(result.Rules))
	for _, r := range result.Rules {
		rules = append(rules, r)
	}
	if err := db.WriteRules(tx, rules); err != nil {
		return fmt.Errorf("write rules: %w", err)
	}

	if err := db.WriteAlerts(tx, result.Alerts); err != nil {
		return fmt.Errorf("write alerts: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	logger.Info("transformation completed successfully",
		"db", db.Path(),
		"total_repositories", len(result.Repositories),
		"total_rules", len(result.Rules),
		"total_alerts", len(result.Alerts))

	return nil
}
