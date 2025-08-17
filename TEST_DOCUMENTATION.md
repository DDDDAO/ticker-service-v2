# Test Documentation for Ticker Service v2

This document describes all tests in the ticker-service-v2 codebase. Each test includes detailed descriptions explaining what is being tested and why it matters for the system's behavior.

## Overview

The tests are organized into several categories:
1. **Exchange Handler Tests** - Test exchange-specific behaviors and quirks
2. **Server API Tests** - Test HTTP endpoints and request validation
3. **Manager Tests** - Test orchestration and lifecycle management
4. **Integration Tests** - Test full system behavior with real connections

## Exchange Handler Tests

### Binance Tests (`internal/exchanges/binance_test.go`)

#### TestNewBinanceHandler
**Purpose**: Verifies Binance handler initialization with proper symbol parsing
- Tests symbol normalization (btcusdt@ticker → BTCUSDT)
- Tests case-insensitive handling
- Ensures backward compatibility with old config formats

#### TestBinanceSubscribeUnsubscribe
**Purpose**: Tests Binance's unique dynamic subscription capability
- Binance is the ONLY exchange supporting dynamic subscription
- Uses !miniTicker@arr stream for all symbols
- Filters to subscribed symbols client-side
- No reconnection needed for subscribe/unsubscribe

#### TestNormalizeBinanceSymbol
**Purpose**: Tests symbol format conversion
- Binance format: BTCUSDT (no separator)
- Internal format: BTC/USDT (slash for Redis)
- Critical for Redis key consistency

#### TestParseFloat
**Purpose**: Tests handling of Binance's mixed JSON types
- **Critical bug fix**: Binance sends numbers as both strings and floats
- Example: "50000.50" vs 50000.50
- Function must handle strings, floats, ints, and nil gracefully

### OKX Tests (`internal/exchanges/okx_test.go`)

#### TestOKXSubscriptionLimitations
**Purpose**: Documents why PEPE wasn't found on OKX
- **NO dynamic subscription support**
- All symbols must be in config.yaml
- Subscribe() and Unsubscribe() are no-ops
- This is why user got "Ticker not found" for PEPE

#### TestOKXSymbolFormat
**Purpose**: Tests OKX's dash-separated format
- OKX format: BTC-USDT (with dash)
- Different from Binance/Bybit/Bitget
- Must convert to BTC/USDT for Redis

#### TestOKXErrorMessages
**Purpose**: Documents misleading error messages
- "Try again in a few seconds" is wrong for OKX
- Should say "Add to config.yaml and restart"
- Waiting won't help since no auto-subscription

### Bybit Tests (`internal/exchanges/bybit_test.go`)

#### TestBybitDeltaUpdate
**Purpose**: Tests Bybit's delta update mechanism
- **Critical feature**: Bybit only sends changed fields
- Initial snapshot has all fields
- Subsequent updates only have bid/ask
- **Must cache and merge** to maintain complete data
- Without caching: missing last/high/low/volume

#### TestBybitReconnection
**Purpose**: Tests cache persistence across reconnections
- Cache survives disconnections
- Prevents data loss during network issues
- Must re-subscribe all symbols on reconnect

### Bitget Tests (`internal/exchanges/bitget_test.go`)

#### TestBitgetFieldMapping
**Purpose**: Documents Bitget's non-standard field names
- **Critical bug**: Wrong field names = all zeros
- Correct mapping:
  - `last` → `lastPr`
  - `bid` → `bidPr`
  - `ask` → `askPr`
  - `high` → `high24h`
  - `low` → `low24h`
  - `volume` → `baseVolume`

#### TestBitgetSubscriptionModel
**Purpose**: Documents static subscription requirement
- No dynamic subscription
- All symbols in config.yaml
- Popular symbols should be pre-configured

### Gate.io Tests (`internal/exchanges/gate_test.go`)

#### TestGateSymbolFormat
**Purpose**: Tests Gate's underscore format
- Gate format: BTC_USDT (underscore)
- Unique among all exchanges
- Symbol format comparison table included

#### TestGateDataCompleteness
**Purpose**: Documents Gate's superior data quality
- Provides complete data in every message
- No caching needed (unlike Bybit)
- Includes bonus fields (change percentage, quote volume)

## Server API Tests

### TestTickerEndpointSymbolValidation
**Purpose**: Tests strict API format enforcement
- Valid: `btc-usdt`, `BTC-USDT`
- Invalid: `btc`, `btcusdt`, `BTC/USDT`
- Enforces `{base}-{quote}` format
- User feedback: "don't make it too friendly"

### TestAutoSubscriptionBehavior
**Purpose**: Documents per-exchange subscription behavior
- **Binance**: Auto-subscribes, waits 2 seconds
- **Others**: Return error immediately
- Clear documentation of which exchanges support what

### TestHealthEndpoint
**Purpose**: Tests health check format
- Checks Redis connectivity
- Returns 200 OK or 503 Unavailable
- JSON format for monitoring tools

### TestStatusEndpoint
**Purpose**: Documents status response format
- Shows all exchange connection states
- Includes message counts, errors, reconnects
- Used for monitoring and debugging

## Manager Tests

### TestManagerAddExchange
**Purpose**: Tests exchange registration
- Prevents duplicate exchanges
- Tracks all registered exchanges
- Thread-safe registration

### TestManagerPublishTicker
**Purpose**: Tests data flow pipeline
- Exchange → Manager → Redis
- Pub/sub channel: `ticker:{exchange}:{symbol}`
- Storage key: `ticker:latest:{exchange}:{symbol}`
- Latency tracking

### TestManagerConcurrency
**Purpose**: Tests thread safety
- Concurrent exchange registration
- Concurrent status queries
- Proper mutex usage

### TestManagerLifecycle
**Purpose**: Tests Start/Stop lifecycle
- Start() connects all handlers
- Context cancellation stops all
- Wait() blocks until completion

## Integration Tests

### TestBinanceMiniTickerIntegration
**Purpose**: Full Binance flow test
- Connects to real WebSocket
- Tests !miniTicker@arr stream
- Verifies filtering and normalization
- Tests dynamic subscription

### TestLatencyMeasurement
**Purpose**: Documents latency fix
- **Bug**: Exchange timestamps are 1+ seconds old
- **Fix**: Use receive time, not exchange time
- Result: Accurate <50ms latency

### TestSymbolFormatValidation
**Purpose**: Documents all format requirements
- Valid vs invalid formats
- Internal conversion chain
- Format consistency across exchanges

### TestExchangeSpecificBehaviors
**Purpose**: Reference documentation
- Summary of each exchange's quirks
- Dynamic vs static subscription
- Symbol format differences
- Field name variations

### TestRedisDataStructure
**Purpose**: Documents Redis storage
- Channel format for pub/sub
- Key format for storage
- JSON structure
- TTL settings

## Key Insights from Tests

### Critical Bug Fixes Documented
1. **Binance**: Mixed JSON types (string vs number)
2. **Bybit**: Delta updates requiring caching
3. **Bitget**: Wrong field names (lastPr not last)
4. **Latency**: Using receive time not exchange time

### Exchange Subscription Models
- **Dynamic** (can add/remove symbols): Binance only
- **Static** (config.yaml only): OKX, Bybit, Bitget, Gate

### Symbol Format Summary
| Exchange | API Input | Exchange Format | Redis Storage |
|----------|-----------|-----------------|---------------|
| API      | btc-usdt  | -               | -             |
| Binance  | btc-usdt  | BTCUSDT         | BTC/USDT      |
| OKX      | btc-usdt  | BTC-USDT        | BTC/USDT      |
| Bybit    | btc-usdt  | BTCUSDT         | BTC/USDT      |
| Bitget   | btc-usdt  | BTCUSDT         | BTC/USDT      |
| Gate     | btc-usdt  | BTC_USDT        | BTC/USDT      |

### Common Issues and Solutions
1. **"PEPE not found on OKX"** - Add to config.yaml
2. **"High latency warnings"** - Fixed by using receive time
3. **"Bybit missing data"** - Implemented state caching
4. **"Bitget shows zeros"** - Fixed field names

## Running Tests

```bash
# Run all tests with verbose output
go test -v ./...

# Run specific package tests
go test -v ./internal/exchanges/...
go test -v ./internal/server/...

# Run with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run integration tests (requires internet and Redis)
go test -tags=integration -v ./...
```

## Test Philosophy

These tests serve multiple purposes:
1. **Verification** - Ensure code works correctly
2. **Documentation** - Explain system behavior
3. **Regression Prevention** - Catch bugs early
4. **Knowledge Transfer** - Help new developers understand the system

Each test includes detailed descriptions explaining:
- What is being tested
- Why it matters
- What bugs were fixed
- What behavior users can expect

This makes the tests valuable not just for validation but as living documentation of the system's behavior and requirements.