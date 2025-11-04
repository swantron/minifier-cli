package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "minifier-cli",
	Short: "A tool to minify container images based on runtime file access tracing",
	Long: `minifier-cli is an open-source (Apache 2.0) tool written in Go that helps create
minimal container images by tracing file access patterns during runtime and repackaging
only the necessary files.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(traceCmd)
	rootCmd.AddCommand(repackageCmd)
}
