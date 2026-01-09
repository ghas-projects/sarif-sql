package cmd

import (
	"fmt"
	"os"

	"github.com/ghas-projects/sarif-avro/cmd/analysis"
	"github.com/ghas-projects/sarif-avro/cmd/transform"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "sarif-avro",
	Short: "A tool to convert SARIF files to Avro format",
	Long:  `sarif-avro is a command-line tool that converts SARIF (Static Analysis Results Interchange Format) files into Avro format for easier data processing and analysis.`,
}

func init() {
	rootCmd.AddCommand(analysis.AnalysisCmd)
	rootCmd.AddCommand(transform.TransformCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
