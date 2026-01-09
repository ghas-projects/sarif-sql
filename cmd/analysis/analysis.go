package analysis

import "github.com/spf13/cobra"

var AnalysisCmd = &cobra.Command{
	Use:   "analysis",
	Short: "Manage MRVA analysis lifecycle and SARIF artifacts",
	Long: `The analysis command manages the lifecycle of MRVA analyses.
It allows users to initialize analysis directories, query analysis
status, and download SARIF artifacts into a structured layout.`,
}

func init() {
	AnalysisCmd.AddCommand(AnalysisStartCmd)
	AnalysisCmd.AddCommand(AnalysisStatusCmd)
	AnalysisCmd.AddCommand(AnalysisDownloadCmd)
}
