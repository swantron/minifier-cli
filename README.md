# minifier-cli

A tool to create minimal container images by tracing runtime file access patterns and repackaging only the necessary files.

## Overview

`minifier-cli` is an open-source (Apache 2.0) tool written in Go that helps reduce container image sizes by:

1. **Tracing** - Running your container with eBPF tracing to capture all file accesses
2. **Analyzing** - Identifying dependencies and required files based on trace data
3. **Repackaging** - Building a new minimal image containing only what's needed

This is particularly useful for hardening appliances like `datadog-agent` and other complex containerized applications.

## Installation

### Download Binary

Download the latest release for your platform from the [releases page](https://github.com/swantron/minifier-cli/releases).

### Build from Source

```bash
git clone https://github.com/swantron/minifier-cli.git
cd minifier-cli
go build -o minifier-cli .
```

## Usage

### Interactive Mode (v0.1)

The tool uses a stateful, multi-step workflow:

#### 1. Start Tracing

```bash
minifier-cli trace start \
  --image datadog/agent:latest \
  --name dd-agent \
  -e DD_API_KEY=your-key
```

This command:
- Launches the container with its default ENTRYPOINT
- Attaches eBPF tracer to capture file accesses
- Writes traced files to `/tmp/minifier-trace-dd-agent.log`
- Runs in the background (daemonized)

#### 2. Exercise the Application

Manually interact with your running container to generate trace data:
- Send mock data to the agent
- Hit health check endpoints
- Configure features
- Exercise all functionality you need in production

**⚠️ WARNING**: Your minified image will ONLY contain files accessed during this manual testing phase. Make sure to exercise every feature you intend to use.

#### 3. Stop Tracing

```bash
minifier-cli trace stop --name dd-agent
```

This stops the eBPF tracer and the container, finalizing the trace log.

#### 4. Repackage

```bash
minifier-cli repackage \
  --name dd-agent \
  --output my-hardened-agent:latest
```

This reads the trace log, analyzes dependencies, and builds a new minified image.

### Advanced Options

You can also use a trace log directly:

```bash
minifier-cli repackage \
  --log-file /path/to/trace.log \
  --output minified-image:latest
```

## How It Works

1. **eBPF Tracing**: Attaches to kernel syscalls (`sys_enter_openat`, `sys_enter_execve`) to capture all file accesses
2. **Dependency Resolution**: Resolves symlinks, ELF binary dependencies, and adds essential system files
3. **Image Repackaging**: Copies traced files from the original image and builds a new minimal image from scratch

## Architecture

```
minifier-cli/
├── cmd/              # CLI commands (cobra)
│   ├── root.go       # Root command setup
│   ├── trace.go      # trace start/stop commands
│   └── repackage.go  # repackage command
├── pkg/
│   ├── session/      # Session state management
│   ├── tracer/       # eBPF tracer implementation
│   ├── analyzer/     # Dependency analysis engine
│   └── repackager/   # Docker image builder
└── main.go           # Entry point
```

## Requirements

- Docker installed and running
- Linux kernel with eBPF support (for tracing)
- `CAP_SYS_ADMIN` and `CAP_SYS_PTRACE` capabilities

## Limitations

- **v0.1** does not support Java/JVM applications
- Requires thorough manual testing to ensure complete coverage
- Only captures file accesses during the trace period

## Future Roadmap

- **v0.2**: CI/Test-Suite Mode - Automated tracing with test suites
- Recipe files for common applications (e.g., `datadog.recipe.sh`)
- Enhanced ELF dependency resolution
- Support for more runtime environments

## License

Apache License 2.0 - See [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Testing

### Quick Test

Run the automated demo:

```bash
./demo.sh
```

Or run the full test suite:

```bash
./test.sh
```

See [TESTING.md](TESTING.md) for detailed testing instructions and scenarios.

### Run Unit Tests

```bash
go test ./...
```
