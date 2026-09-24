package agentcore

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
	"github.com/vanpiyp/awp/internal/paths"
)

type CompactionSettings struct {
	Enabled          bool
	ReserveTokens    int
	KeepRecentTurns  int
	MaxContextTokens int
	Proactive        bool
	Semantic         bool
}

type Config struct {
	Model string
}

func LoadConfig() (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("AW")
	v.AutomaticEnv()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(paths.Home())

	v.SetDefault("model", "MiniMax-M3")

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	cfg := &Config{
		Model: v.GetString("model"),
	}

	return cfg, nil
}
