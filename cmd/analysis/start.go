package analysis

import (
	"github.com/spf13/cobra"
)

var AnalysisStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Initialize analysis directories",
	Long: `The start command initializes the local folder structure for one or
			more MRVA analyses. For each analysis ID provided, a directory is
			created using the format:

			analysis/{id}/

			This prepares the workspace for storing SARIF results and related
			analysis artifacts.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		node := cmd
		if node.HasParent() {
			node = node.Parent()
			if node.PersistentPreRunE != nil {
				if err := node.PersistentPreRunE(cmd, args); err != nil {
					return err
				}
			}
		}
		return nil
	},

	RunE: func(cmd *cobra.Command, args []string) error {
		return analysisService.StartAnalysis(cmd.Context())
	},
}


