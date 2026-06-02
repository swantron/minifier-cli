# Contributing

Bug reports and pull requests are welcome.

## Development Setup

```bash
git clone https://github.com/swantron/minifier-cli
cd minifier-cli
go build -o minifier-cli .
```

Requirements: Go 1.23+, Docker running, Linux for tracing (macOS works for builds and non-tracing tests).

## Running Tests

```bash
go test ./...
```

Tests that require Docker are skipped automatically if the daemon isn't available. ELF-related tests skip on macOS. See [TESTING.md](TESTING.md) for manual testing scenarios and [TESTS.md](TESTS.md) for full test documentation.

## Code Style

Run before submitting:

```bash
gofmt -s -w .
go vet ./...
```

The CI format check will fail if code isn't gofmt'd.

## Pull Requests

- Keep changes focused — one thing per PR
- Add or update tests for new behavior
- Update documentation if the CLI interface changes
- The tracer only works on Linux; if you're on macOS, document any Linux-specific behavior you can't test locally
