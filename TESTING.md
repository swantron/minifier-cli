# Testing Guide

## Quick Test

Run the automated test script:

```bash
./test.sh
```

This will test basic functionality with an Alpine Linux container.

## Manual Testing

### 1. Simple Container Test

Test with a basic Alpine container:

```bash
# Start tracing
./minifier-cli trace start \
  --image alpine:latest \
  --name test-alpine \
  /bin/sh -c "echo 'Hello World' && ls -la /"

# Wait a few seconds for container to complete
sleep 5

# Stop tracing
./minifier-cli trace stop --name test-alpine

# View the trace log
cat /tmp/minifier-trace-test-alpine.log

# Repackage (if trace log has content)
./minifier-cli repackage \
  --name test-alpine \
  --output minified-alpine:test
```

### 2. Interactive Container Test

Test with an interactive container:

```bash
# Start tracing with a long-running container
./minifier-cli trace start \
  --image nginx:alpine \
  --name test-nginx \
  -p 8080:80

# In another terminal, interact with the container
curl http://localhost:8080
docker exec minifier-test-nginx ls -la /usr/share/nginx/html

# Stop tracing
./minifier-cli trace stop --name test-nginx

# Check trace log
cat /tmp/minifier-trace-test-nginx.log

# Repackage
./minifier-cli repackage \
  --name test-nginx \
  --output minified-nginx:test
```

### 3. Test with Custom Log File

Create a manual trace log and test repackaging:

```bash
# Create a trace log
cat > /tmp/test-trace.log << 'EOF'
/bin/bash
/lib/x86_64-linux-gnu/libc.so.6
/usr/bin/ls
/etc/passwd
/etc/group
EOF

# Repackage using the log file
./minifier-cli repackage \
  --log-file /tmp/test-trace.log \
  --output test-output:latest
```

## Expected Behavior

### What Works

- CLI command parsing and help text
- Session management (save/load/delete)
- Docker container lifecycle (start/stop)
- Live file access tracing via `/proc/fd` polling
- ELF binary dependency analysis and recursive resolution
- Symlink resolution
- Repackager with full Docker metadata preservation

### Testing on Linux

The tracer requires Linux (it reads `/proc` from within the container). Run with the required capabilities:

```bash
sudo ./minifier-cli trace start \
  --image your-image:tag \
  --name test-session \
  --cap-add SYS_ADMIN \
  --cap-add SYS_PTRACE
```

## Troubleshooting

### Docker not running
```
Error: Docker is not running
```
**Solution:** Start Docker Desktop or Docker daemon

### Session not found
```
Error loading session: session 'test' not found
```
**Solution:** Check `/tmp/minifier-session-*.json` files or start a new session

### Container already exists
```
Error: Conflict. The container name "/minifier-test" is already in use
```
**Solution:** Stop and remove the existing session:
```bash
./minifier-cli trace stop --name test
# or manually:
docker rm -f minifier-test
```

### Empty trace log
If the trace log is empty, ensure the tracer ran long enough for the container to access files, and that the container was running (not immediately exiting). You can also supply a manually created trace log via `--log-file`.

## CI/CD Testing

The GitHub Actions workflow automatically:
- Builds for multiple platforms (Linux, macOS, Windows)
- Runs unit tests on all packages
- Creates release binaries on tagged commits

View the workflow status at:
https://github.com/swantron/minifier-cli/actions
