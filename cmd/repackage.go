package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/swantron/minifier-cli/pkg/analyzer"
	"github.com/swantron/minifier-cli/pkg/repackager"
	"github.com/swantron/minifier-cli/pkg/session"
)

var repackageCmd = &cobra.Command{
	Use:   "repackage --name <session-name> --output <new-image:tag>",
	Short: "Repackage a traced container into a minified image",
	Long: `Read the trace log, analyze dependencies, and build a new minified container image
containing only the files that were accessed during the trace session.`,
	Run: runRepackage,
}

var (
	outputImage     string
	logFile         string
	sourceImageFlag string
)

func init() {
	repackageCmd.Flags().StringVar(&sessionName, "name", "", "Session name (mutually exclusive with --log-file)")
	repackageCmd.Flags().StringVar(&logFile, "log-file", "", "Path to trace log file (mutually exclusive with --name)")
	repackageCmd.Flags().StringVar(&sourceImageFlag, "source-image", "", "Source image to copy files from (required with --log-file)")
	repackageCmd.Flags().StringVar(&outputImage, "output", "", "Output image name:tag (required)")
	repackageCmd.MarkFlagRequired("output")
}

func runRepackage(cmd *cobra.Command, args []string) {
	if sessionName == "" && logFile == "" {
		fmt.Fprintln(os.Stderr, "Error: Either --name or --log-file must be specified")
		os.Exit(1)
	}

	if sessionName != "" && logFile != "" {
		fmt.Fprintln(os.Stderr, "Error: --name and --log-file are mutually exclusive")
		os.Exit(1)
	}

	if logFile != "" && sourceImageFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: --source-image is required when using --log-file")
		os.Exit(1)
	}

	traceLogPath := logFile
	var sourceImage string

	if sessionName != "" {
		sess, err := session.Load(sessionName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading session: %v\n", err)
			os.Exit(1)
		}
		traceLogPath = sess.LogFile
		sourceImage = sess.Image
	} else {
		sourceImage = sourceImageFlag
	}

	a := analyzer.NewAnalyzer()
	manifest, err := a.Analyze(traceLogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error analyzing trace log: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Analyzed %d unique files from trace log\n", len(manifest.Files))

	r := repackager.NewRepackager()
	if err := r.Repackage(sourceImage, outputImage, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error repackaging image: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully created minified image: %s\n", outputImage)
}
