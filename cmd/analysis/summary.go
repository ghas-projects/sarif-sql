package analysis

import (
	"github.com/spf13/cobra"
)

var AnalysisSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Check MRVA analysis summary",
	Long: `The summary command retrieves and displays the current state of one
			or more MRVA analyses. This allows users to monitor analysis progress
			and determine when results are ready to be downloaded.`,
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
		return analysisService.GetAnalysisSummary(cmd.Context())
	},
}
