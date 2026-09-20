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
	Enabled         bool
	ReserveTokens   int
	KeepRecentTurns int
}

type Config struct {
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

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	cfg := &Config{
		Model:   v.GetString("model"),
		LogPath: v.GetString("log_path"),
	}

	if err := v.UnmarshalKey("tools", &cfg.Tools); err != nil {
		return nil, fmt.Errorf("unmarshal tools: %w", err)
	}

	return cfg, nil
}
