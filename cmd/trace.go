package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/swantron/minifier-cli/pkg/session"
	"github.com/swantron/minifier-cli/pkg/tracer"
)

var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Manage trace sessions",
	Long:  `Start and stop eBPF tracing sessions for containers.`,
}

var traceStartCmd = &cobra.Command{
	Use:   "start --image <image:tag> --name <session-name> [docker-run-args...]",
	Short: "Start a trace session",
	Long: `Start a container with eBPF tracing enabled. The container runs with its default
ENTRYPOINT while file access is traced and logged.`,
	Run: runTraceStart,
}

var traceStopCmd = &cobra.Command{
	Use:   "stop --name <session-name>",
	Short: "Stop a trace session",
	Long:  `Stop the eBPF tracer and the associated container, finalizing the trace log.`,
	Run:   runTraceStop,
}

var traceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active trace sessions",
	Long:  `List all trace sessions currently saved in the temp directory.`,
	Run:   runTraceList,
}

var (
	imageName    string
	sessionName  string
	dockerArgs   []string
	traceTimeout time.Duration
)

func init() {
	traceStartCmd.Flags().StringVar(&imageName, "image", "", "Container image to trace (required)")
	traceStartCmd.Flags().StringVar(&sessionName, "name", "", "Session name for this trace (required)")
	traceStartCmd.Flags().DurationVar(&traceTimeout, "timeout", 5*time.Minute, "Maximum trace duration")
	_ = traceStartCmd.MarkFlagRequired("image")
	_ = traceStartCmd.MarkFlagRequired("name")

	traceStopCmd.Flags().StringVar(&sessionName, "name", "", "Session name to stop (required)")
	_ = traceStopCmd.MarkFlagRequired("name")

	traceCmd.AddCommand(traceStartCmd)
	traceCmd.AddCommand(traceStopCmd)
	traceCmd.AddCommand(traceListCmd)
}

func runTraceStart(cmd *cobra.Command, args []string) {
	dockerArgs = args

	t := tracer.NewTracerWithTimeout(traceTimeout)
	sess, done, err := t.Start(imageName, sessionName, dockerArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting trace: %v\n", err)
		os.Exit(1)
	}

	if err := session.Save(sess); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving session: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Container %s started for trace session '%s'\n", sess.ContainerID, sess.Name)
	fmt.Printf("Trace log: %s\n", sess.LogFile)
	fmt.Printf("\nTracer is running. Use 'trace stop --name %s' to stop.\n", sess.Name)
	fmt.Printf("Or press Ctrl+C to stop tracing (container will keep running).\n")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-done:
		fmt.Println("Tracer finished.")
	case sig := <-sigCh:
		fmt.Printf("\nReceived %v, stopping tracer (container still running).\n", sig)
	}
}

func runTraceList(cmd *cobra.Command, args []string) {
	sessions, err := session.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing sessions: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Println("No active trace sessions.")
		return
	}

	fmt.Printf("%-20s  %-35s  %-13s  %s\n", "NAME", "IMAGE", "CONTAINER", "LOG FILE")
	fmt.Printf("%-20s  %-35s  %-13s  %s\n", "----", "-----", "---------", "--------")
	for _, sess := range sessions {
		container := sess.ContainerID
		if len(container) > 12 {
			container = container[:12]
		}
		image := sess.Image
		if len(image) > 35 {
			image = image[:32] + "..."
		}
		fmt.Printf("%-20s  %-35s  %-13s  %s\n", sess.Name, image, container, sess.LogFile)
	}
}

func runTraceStop(cmd *cobra.Command, args []string) {
	sess, err := session.Load(sessionName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading session: %v\n", err)
		os.Exit(1)
	}

	t := tracer.NewTracer()
	if err := t.Stop(sess); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping trace: %v\n", err)
		os.Exit(1)
	}

	if err := session.Delete(sessionName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not delete session file: %v\n", err)
	}

	fmt.Printf("Trace session '%s' stopped. Log file at %s\n", sess.Name, sess.LogFile)
}
