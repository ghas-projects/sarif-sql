package analysis

import (
	"fmt"

	"github.com/spf13/cobra"
)

var AnalysisDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download SARIF results for an analysis",
	Long: `The download command fetches SARIF result files for completed MRVA
			analyses and stores them in the corresponding local analysis
			directories (analysis/{id}/).

			Only available SARIF artifacts will be downloaded.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Downloading SARIF artifacts...")
		// TODO: Implement download logic
	},
}
