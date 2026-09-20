package agent

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
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

	Model    string
	MaxTurns int
	LogPath  string

	Tools []ToolSpec

	Compaction CompactionSettings
}

func LoadConfig() (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("AW")
	v.AutomaticEnv()
	v.SetConfigName("aw")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./configs")

	v.SetDefault("model", "MiniMax-M3")
	v.SetDefault("max_turns", 10)
	v.SetDefault("log_path", "")
	v.SetDefault("tools", []map[string]any{})
	v.SetDefault("compaction.enabled", true)
	v.SetDefault("compaction.reserve_tokens", 16384)
	v.SetDefault("compaction.keep_recent_turns", 5)
	v.SetDefault("compaction.compact_every_turns", 3)
	v.SetDefault("compaction.min_turns_between", 5)
	v.SetDefault("compaction.floor_percent", 40)

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
		APIKey:   v.GetString("api_key"),
		Model:    v.GetString("model"),
		MaxTurns: v.GetInt("max_turns"),
		LogPath:  v.GetString("log_path"),
		Compaction: CompactionSettings{
			Enabled:            v.GetBool("compaction.enabled"),
			ReserveTokens:      v.GetInt("compaction.reserve_tokens"),
			KeepRecentTurns:    v.GetInt("compaction.keep_recent_turns"),
			CompactEveryTurns:  v.GetInt("compaction.compact_every_turns"),
			MinTurnsBetween:    v.GetInt("compaction.min_turns_between"),
			FloorPercent:       v.GetInt("compaction.floor_percent"),
		},
	}

	if cfg.APIKey == "" {
		return nil, errors.New("MINIMAX_API_KEY is required")
	}

	if err := v.UnmarshalKey("tools", &cfg.Tools); err != nil {
		return nil, fmt.Errorf("unmarshal tools: %w", err)
	}

	return cfg, nil
}
