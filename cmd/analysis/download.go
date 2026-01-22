package analysis

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	directory string
)

var AnalysisDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download result artifacts for an analysis",
	Long: `The download command fetches SARIF result files for completed MRVA
			analyses and stores them in the corresponding local analysis
			directories (analysis/{id}/).

			Only available SARIF artifacts will be downloaded.`,
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
		fmt.Println("Downloading artifacts...")
		return analysisService.DownloadAnalysiFiles(cmd.Context(), directory)
	},
}

func init() {
	AnalysisDownloadCmd.PersistentFlags().StringVar(&directory, "directory", "", "Specify the MRVA analysis ID")
	AnalysisDownloadCmd.MarkPersistentFlagRequired("directory")

}
