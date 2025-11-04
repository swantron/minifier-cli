# Technical Implementation Plan
**Project:** minifier-cli (Open-Core Tool v0.1)
**Status:** DRAFT (Revised for "Appliance Hardening")

## 1. Overview & Revised Workflow
This document describes the technical implementation for `minifier-cli`, an open-source (Apache 2.0) tool written in Go.
The tool's core function has been updated to support two modes. The v0.1 priority is the "Interactive/Daemon Mode" required for hardening appliances like `datadog-agent`.

### v0.1 Workflow: "Interactive/Daemon Mode"
Instead of a single command, the user will now use a stateful, multi-step process:

* `minifier-cli trace start ...`
    * This command launches the target container (e.g., `datadog-agent`) with its normal ENTRYPOINT.
    * It simultaneously attaches the eBPF tracer.
    * It writes all traced file paths to a persistent "trace log" (e.g., `/tmp/minifier-trace-dd-agent.log`).
    * The command then exits, leaving the container and the trace running in the background (daemonized).

* **[Manual User Step]**
    * The user now interacts with the running agent. They send it mock data, hit its health check endpoints, configure it, etc. This is the "manual testing" phase that generates the trace data.

* `minifier-cli trace stop ...`
    * This command stops the eBPF trace, gracefully stops the running container, and finalizes the trace log.

* `minifier-cli repackage ...`
    * This command reads the completed trace log, runs the Analysis Engine, and builds the new, minified image.

### v0.2 Workflow: "CI/Test-Suite Mode"
This is the workflow from the previous plan. It's now a lower priority, but we can see it's just a wrapper around the v0.1 commands:

* `minifier-cli run --image ... --command "pytest" ...` (This command will internally call `trace start`, wait for the `--command` to exit, call `trace stop`, and then call `repackage`, all in one go.)

## 2. Revised CLI Commands (v0.1)
The tool will use subcommands (cobra) to manage state, using a name (e.g., `my-dd-trace`) to identify a session.

* `minifier-cli trace start --image <image:tag> --name <session-name> [docker-run-args...]`
    * Starts the container from `<image:tag>` with its default ENTRYPOINT.
    * Applies required eBPF capabilities (`--cap-add SYS_ADMIN`, etc.).
    * Allows passthrough of other `docker run` args (like `-e API_KEY`, `-v /var/run/docker.sock`).
    * Begins tracing and saves data to `/tmp/minifier-trace-<session-name>.log`.
    * Prints `Container <cid> started for trace session '<session-name>'`.

* `minifier-cli trace stop --name <session-name>`
    * Finds the container and trace process associated with `<session-name>`.
    * Stops the tracer, stops the container, and removes it.
    * Prints `Trace session '<session-name>' stopped. Log file at /tmp/minifier-trace-<session-name>.log`.

* `minifier-cli repackage --name <session-name> --output <new-image:tag>`
    * This command is stateless. It only needs the log file.
    * It can also be run with `--log-file /path/to/trace.log` for more manual control.
    * This command executes the Analysis Engine and Repackager steps exactly as described in the previous plan.

## 3. Core Components (No Change)
The internal components are the same, but their orchestration is different.

* **The CLI (cobra):** Now manages the stateful `start`/`stop`/`repackage` workflow.
* **The Tracer (ebpf):** Runs as a daemon process, writing to a file instead of an in-memory map.
* **The Analysis Engine (go):** Unchanged. It now reads from the trace log file instead of an in-memory map.
* **The Repackager (docker):** Unchanged. It's triggered by the `repackage` command.

## 4. Architecture & Data Flow (Revised)

* **[CLI]** User runs `minifier-cli trace start --image datadog/agent:latest --name dd-agent -e DD_API_KEY=....`
* **[CLI]** The tool generates a unique container name, adds the eBPF capabilities, and starts the container.
* **[Tracer]** A separate, daemonized Go process is launched. It:
    * Loads the eBPF program into the kernel.
    * Attaches to `sys_enter_openat` and `sys_enter_execve`.
    * Filters for PIDs from the target container.
    * Reads from the BPF ring buffer.
    * **New Step:** Appends each unique file path (as a simple text line) to `/tmp/minifier-trace-dd-agent.log`.
* **[USER]** User spends 10 minutes sending mock stats to the agent, accessing its web UI, etc. The log file grows.
* **[CLI]** User runs `minifier-cli trace stop --name dd-agent`.
* **[CLI]** The tool finds the daemonized tracer process, sends it a `SIGTERM`, waits for it to clean up, and then stops and removes the `datadog/agent` container.
* **[CLI]** User runs `minifier-cli repackage --name dd-agent --output my-hardened-agent:latest`.
* **[Analysis Engine]** The engine reads `/tmp/minifier-trace-dd-agent.log` into a `map[string]struct{}`.
* **[Analysis Engine]** It performs the exact same Dependency Resolution as in the previous plan (symlinks, ELF parsing, safelist injection).
* **[Analysis Engine]** It outputs the final "File Manifest."
* **[Repackager]** It performs the exact same repackaging steps:
    * Start temporary builder container.
    * Copy all files from the Manifest into a local temp dir.
    * Generate `Dockerfile.minified` with original metadata.
    * Run `docker build`.
    * Clean up.
* **[CLI]** The tool prints the final report.

## 5. Key Technical Challenges & Risks (Updated)

### The "Manual Coverage" Problem (HIGH RISK)
This is our biggest risk, and your new use case makes it front-and-center.
* If the user, during their manual testing, never triggers the agent's alerting feature, the binaries and files for alerting will not be in the trace log.
* The minified agent will then fail silently in production when an alert tries to fire.
* **Mitigation (v0.1):** This is a pure documentation and user education problem. We must be brutally clear: "WARNING: Your minified appliance is ONLY as complete as your manual testing. You must exercise every single feature you intend to use."
* **Mitigation (v0.2):** We can provide "recipe" files. For example, a `datadog.recipe.sh` script that runs a 5-minute automated test against the agent, hitting all known endpoints to generate a good baseline trace.

### State Management
We now need to manage the state of running traces. What if the user's machine reboots?
* **Mitigation (v0.1):** We won't. If the trace process is killed, the user has to stop the container and start over. We will store the PID of the tracer and the container ID in a simple file (e.g., `/tmp/minifier-session-dd-agent.json`) to allow the `stop` command to find them.

### Java (The JVM)
This risk is unchanged.
* **Mitigation (v0.1):** We will not support Java in v0.1. This use case doesn't change that. We will focus on Python, Node, Go, and (like Datadog agent) pre-compiled binaries.
