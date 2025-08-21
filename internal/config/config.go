package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig              `mapstructure:"server"`
	Redis     RedisConfig               `mapstructure:"redis"`
	Exchanges map[string]ExchangeConfig `mapstructure:"exchanges"`
	Logging   LoggingConfig             `mapstructure:"logging"`
}

type ServerConfig struct {
	Port                      int   `mapstructure:"port"`
	ProcessingLatencyWarnMs   int64 `mapstructure:"processing_latency_warn_ms"`
	StoreLatencyWarnMs        int64 `mapstructure:"store_latency_warn_ms"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type ExchangeConfig struct {
	Enabled bool     `mapstructure:"enabled"`
	Symbols []string `mapstructure:"symbols"`
	WSURL   string   `mapstructure:"ws_url"`
	ApiKey  string   `mapstructure:"api_key"`
	Secret  string   `mapstructure:"secret"`
}

type LoggingConfig struct {
	Level          string `mapstructure:"level"`
	Format         string `mapstructure:"format"`
	Production     bool   `mapstructure:"production"`     // Production mode - only errors
	TickerUpdates  bool   `mapstructure:"ticker_updates"`  // Log each ticker update (verbose)
}

func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.processing_latency_warn_ms", 50)
	v.SetDefault("server.store_latency_warn_ms", 20)
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.db", 0)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.production", false)
	v.SetDefault("logging.ticker_updates", false)

	// Default exchange configurations
	v.SetDefault("exchanges.binance.enabled", true)
	v.SetDefault("exchanges.binance.ws_url", "wss://stream.binance.com:9443/ws")
	v.SetDefault("exchanges.binance.symbols", []string{"btcusdt@ticker", "ethusdt@ticker"})

	v.SetDefault("exchanges.okx.enabled", true)
	v.SetDefault("exchanges.okx.ws_url", "wss://ws.okx.com:8443/ws/v5/public")
	v.SetDefault("exchanges.okx.symbols", []string{"BTC-USDT", "ETH-USDT"})

	v.SetDefault("exchanges.bybit.enabled", true)
	v.SetDefault("exchanges.bybit.ws_url", "wss://stream.bybit.com/v5/public/linear")
	v.SetDefault("exchanges.bybit.symbols", []string{"BTCUSDT", "ETHUSDT"})

	v.SetDefault("exchanges.bitget.enabled", true)
	v.SetDefault("exchanges.bitget.ws_url", "wss://ws.bitget.com/v2/ws/public")
	v.SetDefault("exchanges.bitget.symbols", []string{"BTCUSDT", "ETHUSDT"})

	v.SetDefault("exchanges.gate.enabled", true)
	v.SetDefault("exchanges.gate.ws_url", "wss://api.gateio.ws/ws/v4/")
	v.SetDefault("exchanges.gate.symbols", []string{"BTC_USDT", "ETH_USDT"})

	// Read from environment variables
	v.SetEnvPrefix("TICKER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file if provided
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}