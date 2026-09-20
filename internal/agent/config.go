package agent

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
	"github.com/vanpiyp/awp/internal/storage"
)

type ToolSpec struct {
	Name        string         `mapstructure:"name"`
	Description string         `mapstructure:"description"`
	Parameters  map[string]any `mapstructure:"parameters"`
}

type CompactionSettings struct {
	Enabled            bool
	ReserveTokens      int
	KeepRecentTurns    int
	CompactEveryTurns  int
	MinTurnsBetween    int
	FloorPercent       int
}

type Config struct {
	APIKey string

	Model   string
	LogPath string

	Tools []ToolSpec
}

func LoadConfig() (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("AW")
	v.AutomaticEnv()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(storage.Home())

	v.SetDefault("model", "MiniMax-M3")
	v.SetDefault("log_path", "")
	v.SetDefault("tools", []map[string]any{})

	if err := v.BindEnv("api_key", "MINIMAX_API_KEY"); err != nil {
		return nil, fmt.Errorf("bind env: %w", err)
	}
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	cfg := &Config{
		APIKey:  v.GetString("api_key"),
		Model:   v.GetString("model"),
		LogPath: v.GetString("log_path"),
	}

	if cfg.APIKey == "" {
		return nil, errors.New("MINIMAX_API_KEY is required")
	}

	if err := v.UnmarshalKey("tools", &cfg.Tools); err != nil {
		return nil, fmt.Errorf("unmarshal tools: %w", err)
	}

	return cfg, nil
}
