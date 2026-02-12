package transform

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/ghas-projects/sarif-avro/internal/models"
	"github.com/ghas-projects/sarif-avro/internal/service"
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
  sarif-avro transform --sarif-file analysis.sarif --analysis-id 12345 --repo my-org/my-repo --output ./output

  # Transform all SARIF files in a directory
  sarif-avro transform --sarif-dir ./analyses/12345 --analysis-id 12345 --output ./output`,
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
	TransformCmd.Flags().StringVar(&outputDir, "output", "./proto-output", "Output directory for transformed files")
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

	service := service.NewTransformService(logger, sarifDir, outputDir, analysisID, controllerRepo)

	// Use command context which handles cancellation (Ctrl+C)
	result, err := service.Transform(cmd.Context())
	if err != nil {
		return err
	}
	// Write to Protobuf files
	if err := service.WriteProtoFiles(result); err != nil {
		return fmt.Errorf("failed to write proto files: %w", err)
	}

	logger.Info("Transformation completed successfully",
		"total_repositories", len(result.Repositories),
		"total_rules", len(result.Rules))

	return nil
}
