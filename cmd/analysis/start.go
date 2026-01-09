package analysis

import (
	"fmt"

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
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting analysis...")
		// TODO: Implement start logic
	},
}
