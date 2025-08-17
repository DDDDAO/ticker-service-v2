# Test Organization Guide

## Go Testing Best Practices

### Standard Go Approach (Recommended)

In Go, tests typically live alongside the code they test because:
1. **Access to private functions** - Tests in the same package can test unexported functions
2. **Convention** - Go tooling expects `*_test.go` files next to source files
3. **Easy discovery** - Tests are right next to what they test

```
internal/exchanges/
├── binance.go          # Implementation
├── binance_test.go     # Unit tests (same package)
├── bybit.go
├── bybit_test.go
└── ...
```

### When to Use Separate Test Directories

Use separate directories for:
1. **Integration tests** - Tests that require external services
2. **E2E tests** - End-to-end system tests
3. **Benchmark tests** - Performance testing suites
4. **Large test fixtures** - Test data files

```
tests/
├── integration/        # Integration tests requiring Redis/Internet
│   └── exchange_integration_test.go
├── e2e/               # End-to-end tests
│   └── api_e2e_test.go
├── benchmarks/        # Performance tests
│   └── throughput_bench_test.go
└── fixtures/          # Test data
    └── sample_tickers.json
```

## Current Test Organization

Based on Go best practices, here's the recommended structure:

```
ticker-service-v2/
├── internal/
│   ├── exchanges/
│   │   ├── binance.go
│   │   ├── binance_test.go         # Unit tests
│   │   ├── bybit.go
│   │   ├── bybit_test.go
│   │   └── ...
│   └── server/
│       ├── server.go
│       └── server_test.go          # Unit tests
│
├── tests/                           # Special test categories
│   ├── integration/
│   │   ├── binance_integration_test.go
│   │   ├── full_system_test.go
│   │   └── README.md
│   ├── benchmarks/
│   │   └── websocket_bench_test.go
│   └── testdata/
│       └── mock_responses.json
│
└── TEST_DOCUMENTATION.md            # Test strategy doc
```

## Running Tests

### Run all unit tests
```bash
go test ./...
```

### Run tests in specific package
```bash
go test ./internal/exchanges/...
```

### Run integration tests
```bash
# Integration tests use build tags
go test -tags=integration ./tests/integration/...
```

### Run with coverage
```bash
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Run specific test
```bash
go test -run TestBinanceSubscribe ./internal/exchanges/
```

## Test File Naming Conventions

- `*_test.go` - Standard unit tests
- `*_integration_test.go` - Integration tests (use build tags)
- `*_bench_test.go` - Benchmark tests
- `example_test.go` - Example code for documentation

## Benefits of Keeping Tests with Code

1. **Immediate feedback** - See test right next to implementation
2. **Better maintenance** - Update tests when changing code
3. **Package testing** - Can test private functions
4. **Go tooling** - Works seamlessly with `go test`
5. **IDE support** - Better navigation and test discovery

## When Tests Get Too Large

If test files become too large, you can:
1. Split into multiple test files: `binance_subscription_test.go`, `binance_parsing_test.go`
2. Use test helpers in `testing.go` files
3. Move integration tests to `tests/integration/`
4. Keep fixtures in `testdata/` directories

## Recommendation

For this project, I recommend:
1. **Keep unit tests with source code** (standard Go practice)
2. **Move integration tests to `tests/integration/`**
3. **Create `tests/benchmarks/` for performance tests**
4. **Use `testdata/` directories for test fixtures**

This gives you the best of both worlds:
- Clean separation of concerns
- Access to private functions for unit tests
- Organized special test types
- Follows Go conventions