#!/bin/bash
# Simple demo of minifier-cli commands

set -e

echo "==================================="
echo "  minifier-cli Demo"
echo "==================================="
echo ""

# Check prerequisites
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker."
    exit 1
fi

if [ ! -f "./minifier-cli" ]; then
    echo "Building minifier-cli..."
    go build -o minifier-cli .
    echo ""
fi

echo "1️⃣  Testing CLI Commands"
echo "-----------------------------------"
./minifier-cli --help
echo ""

echo "2️⃣  Testing trace subcommands"
echo "-----------------------------------"
./minifier-cli trace --help
echo ""

echo "3️⃣  Starting a trace session"
echo "-----------------------------------"
SESSION_NAME="demo-$(date +%s)"
docker pull alpine:latest > /dev/null 2>&1

echo "Starting trace for alpine:latest..."
./minifier-cli trace start \
  --image alpine:latest \
  --name $SESSION_NAME

echo ""
echo "✓ Container started!"
echo ""

echo "4️⃣  Checking session files"
echo "-----------------------------------"
SESSION_FILE="/tmp/minifier-session-${SESSION_NAME}.json"
TRACE_LOG="/tmp/minifier-trace-${SESSION_NAME}.log"

if [ -f "$SESSION_FILE" ]; then
    echo "Session file created:"
    cat $SESSION_FILE | jq . 2>/dev/null || cat $SESSION_FILE
else
    echo "⚠️  Session file not found at $SESSION_FILE"
fi
echo ""

if [ -f "$TRACE_LOG" ]; then
    echo "Trace log created: $TRACE_LOG"
    echo "Size: $(wc -l < $TRACE_LOG) lines"
else
    echo "⚠️  Trace log not found (eBPF not implemented yet)"
fi
echo ""

echo "5️⃣  Checking running container"
echo "-----------------------------------"
docker ps | grep "minifier-${SESSION_NAME}" || echo "Container may have exited"
echo ""

sleep 2

echo "6️⃣  Stopping trace session"
echo "-----------------------------------"
./minifier-cli trace stop --name $SESSION_NAME
echo ""

echo "7️⃣  Creating a manual trace log"
echo "-----------------------------------"
MANUAL_LOG="/tmp/manual-trace-demo.log"
cat > $MANUAL_LOG << 'EOF'
/bin/sh
/bin/busybox
/lib/ld-musl-x86_64.so.1
/etc/passwd
/etc/group
/etc/hosts
EOF

echo "Created manual trace log with $(wc -l < $MANUAL_LOG) files"
echo ""

echo "8️⃣  Testing the analyzer"
echo "-----------------------------------"
echo "Analyzing trace log..."
./minifier-cli repackage \
  --log-file $MANUAL_LOG \
  --output demo-minified:test 2>&1 | head -2 || true
echo ""

echo "9️⃣  Cleanup"
echo "-----------------------------------"
rm -f $MANUAL_LOG
echo "✓ Cleaned up test files"
echo ""

echo "==================================="
echo "  ✅ Demo Complete!"
echo "==================================="
echo ""
echo "Next steps:"
echo "  • Read TESTING.md for more test scenarios"
echo "  • Implement eBPF tracing for production use"
echo "  • Test with real applications like nginx or datadog-agent"
echo ""
