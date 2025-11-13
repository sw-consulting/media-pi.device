# Testing Strategy

This document describes the testing approach for the Media Pi device agent.

## Project Structure

The project now follows Go conventions with tests alongside their source code:

```
media-pi.device/
├── go.mod
├── cmd/
│   └── media-pi/
│       ├── main.go           # CLI entry point
│       └── main_test.go      # CLI tests
├── internal/
│   └── agent/
│       ├── agent.go          # Core agent logic
│       ├── dbus.go           # DBus integration
│       ├── menu.go           # Menu handlers
│       ├── *.go             # Other source files
│       ├── *_test.go        # Unit tests (same package)
│       ├── dbus_factory_test.go  # DBus factory tests
│       └── dbus_mock_test.go     # Mock DBus setup
└── test/                     # Integration tests only
    ├── playlist_upload_dbus_test.go  # Real DBus tests
    └── sighup_integration_test.go    # Signal handling tests
```

## Test Categories

### Unit Tests
- Located alongside source code in the same package
- Test internal functions and unexported types
- Fast execution, no external dependencies
- Run with: `go test ./...`

### Integration Tests
- Located in `/test` directory with `//go:build integration` tag
- Test real external dependencies (DBus, signals)
- May require specific hardware or environment
- Run with: `go test -tags=integration ./...`

## Running Tests

### Local Development
```bash
# Run all unit tests
go test -v ./...

# Run unit tests with coverage
go test -v ./... -coverprofile=coverage.out -covermode=atomic

# Run integration tests (requires DBus session)
go test -v -tags=integration ./...

# Run with race detection
go test -race -v ./...
```

### CI/CD
The GitHub Actions workflow runs:
1. Unit tests with coverage on all commits/PRs
2. Integration tests only on main branch pushes
3. Race detection enabled for all test runs

## Test Guidelines

1. **Unit tests**: Keep alongside source code, same package name
2. **Integration tests**: Use build tags `//go:build integration`
3. **Mock external dependencies** for unit tests
4. **Use t.TempDir()** for file system operations
5. **Clean up global state** with defer functions
6. **Skip tests** when dependencies unavailable using `t.Skip()`

## Environment Variables

- `MEDIA_PI_AGENT_CONFIG`: Override config file path for tests
- `MEDIA_PI_AGENT_MOCK_DBUS`: Enable mocked DBus connections
- `ENABLE_REAL_DBUS`: Enable real DBus tests (optional)

## Coverage

Coverage is collected for unit tests and uploaded to Codecov. Integration tests
may be skipped in CI if hardware dependencies are not available.