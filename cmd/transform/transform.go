package transform

import (
	"fmt"

	"github.com/spf13/cobra"
)

var TransformCmd = &cobra.Command{
	Use:   "transform",
	Short: "Transform SARIF files into Avro format",
	Long: `The transform command converts SARIF result files into Avro format
for reporting purposes`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Transforming SARIF files to Avro format...")
	},
}
