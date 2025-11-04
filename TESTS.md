# Test Suite Documentation

## Test Coverage Summary

```
Package         Coverage    Tests   Status
-------------------------------------------------
analyzer        56.2%       12      ✅ All passing
repackager      59.8%       8       ✅ All passing
session         80.0%       3       ✅ All passing
tracer          20.7%       5       ✅ All passing (2 skipped)
-------------------------------------------------
Total           54.2%       28      ✅ All passing
```

## Test Files Created

### 1. `pkg/analyzer/analyzer_test.go` (12 tests)

**Basic Functionality:**
- `TestAnalyzerBasic` - Verifies basic log parsing
- `TestAnalyzerEmptyLog` - Handles empty trace logs
- `TestAnalyzerNonExistentFile` - Error handling for missing files
- `TestAnalyzerDeduplication` - Ensures files aren't duplicated

**ELF Binary Detection:**
- `TestIsELFBinary` - Detects real ELF binaries (skips on macOS)
- `TestIsELFBinaryNonELF` - Rejects non-ELF files

**Symlink Resolution:**
- `TestResolveSymlinks` - Resolves symlinks to real files
- `TestResolveSymlinksNonExistent` - Handles broken symlinks

**Dependency Resolution:**
- `TestResolveELFDependencies` - Finds shared libraries
- `TestGetELFInterpreter` - Extracts dynamic linker from ELF
- `TestAddSafelistFiles` - Verifies safelist is added
- `TestResolveDependencies` - Full dependency tree resolution

### 2. `pkg/repackager/repackager_test.go` (8 tests)

**Metadata Extraction:**
- `TestExtractMetadata` - Parses Docker inspect JSON
- `TestExtractMetadataInvalidImage` - Handles invalid images
- `TestImageMetadataJSONParsing` - Validates JSON struct mapping

**Dockerfile Generation:**
- `TestGenerateDockerfile` - Generates full Dockerfile with all directives
- `TestGenerateDockerfileMinimal` - Minimal Dockerfile with just CMD

**File Operations:**
- `TestCopyFilesEmptyList` - Error handling for empty file lists
- `TestCopyFilesInvalidContainer` - Handles non-existent containers

**Integration:**
- `TestRepackageIntegration` - Full pipeline test (skipped - creates images)

### 3. `pkg/session/session_test.go` (3 tests)

**Session Management:**
- `TestSessionSaveAndLoad` - Save/load session state
- `TestSessionLoadNonExistent` - Error on missing session
- `TestSessionDelete` - Cleanup session files

### 4. `pkg/tracer/tracer_test.go` (5 tests)

**Container Management:**
- `TestTracerGetContainerPID` - Retrieves container PID
- `TestTracerIsContainerRunning` - Checks container status

**File Capture:**
- `TestTracerCaptureBasicFiles` - Finds common files in container
- `TestTracerReadContainerSymlink` - Reads symlinks from container

**Integration:**
- `TestTracerStartStop` - Full trace lifecycle (skipped - long running)

## Running Tests

### All Tests
```bash
go test ./pkg/...
```

### With Verbose Output
```bash
go test -v ./pkg/...
```

### With Coverage
```bash
go test ./pkg/... -cover
```

### Coverage Report
```bash
go test ./pkg/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Specific Package
```bash
go test -v ./pkg/analyzer
go test -v ./pkg/repackager
go test -v ./pkg/session
go test -v ./pkg/tracer
```

## Test Categories

### Unit Tests (Fast)
- All analyzer tests except ELF dependency resolution
- All session tests
- Dockerfile generation tests
- JSON parsing tests

### Integration Tests (Require Docker)
- Metadata extraction tests
- Container PID/status tests
- File capture tests
- Full pipeline tests (skipped by default)

### Platform-Specific Tests
- ELF tests skip on macOS (binaries are Mach-O format)
- Container tests require Docker daemon

## CI/CD Considerations

Tests are designed to run in GitHub Actions:

**Fast tests** (always run):
- Session management
- Dockerfile generation
- JSON parsing
- Basic file operations

**Docker tests** (run when Docker available):
- Metadata extraction
- Container operations
- File capture

**Skipped tests**:
- Full integration tests (create images)
- Long-running trace tests
- ELF tests on non-Linux platforms

## Test Helpers

Each test file includes helper functions:

**analyzer_test.go:**
- `fileExists(path)` - Check if file exists

**repackager_test.go:**
- `isDockerAvailable()` - Check if Docker is running

**tracer_test.go:**
- `isDockerAvailable()` - Check if Docker is running
- `createTestContainer(image)` - Create test container
- `cleanupContainer(id)` - Remove test container
- `tempFile(t)` - Create temporary file

## Future Test Improvements

1. **Mock Docker calls** for unit testing without Docker daemon
2. **Add benchmark tests** for performance-critical paths
3. **Table-driven tests** for edge cases
4. **Increase tracer coverage** with more file capture scenarios
5. **Add E2E tests** that build and run minified images
6. **Property-based testing** for dependency resolution

## Test Failures and Debugging

**Common Issues:**

1. **Docker not available**
   ```
   Solution: Start Docker Desktop or docker daemon
   ```

2. **ELF tests fail on macOS**
   ```
   Solution: Tests automatically skip - this is expected
   ```

3. **Metadata extraction fails**
   ```
   Solution: Ensure alpine:latest is pulled
   docker pull alpine:latest
   ```

4. **Integration tests timeout**
   ```
   Solution: These are skipped by default
   Run manually if needed
   ```

## Adding New Tests

Template for new test:
```go
func TestNewFeature(t *testing.T) {
    // Arrange
    component := NewComponent()
    
    // Act
    result, err := component.Method()
    
    // Assert
    if err != nil {
        t.Fatalf("Unexpected error: %v", err)
    }
    
    if result != expected {
        t.Errorf("Expected %v, got %v", expected, result)
    }
}
```

Remember:
- Use `t.TempDir()` for temporary directories
- Use `t.Skip()` for conditional tests
- Use `t.Parallel()` for parallel-safe tests
- Clean up resources with `defer`
