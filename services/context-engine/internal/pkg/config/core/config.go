package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"go.uber.org/zap"
)

type DefaultConfig struct {
	Config map[string]interface{}
}

func NewConfig(logger *zap.Logger, defaults *DefaultConfig) (config *koanf.Koanf, err error) {
	config = koanf.New(".")
	PREFIX := "SUPERAGI_"

	envKeyTransform := func(s string) string {
		return strings.Replace(
			strings.ToLower(strings.TrimPrefix(s, PREFIX)),
			"_", ".", -1,
		)
	}

	// Layer 1: hardcoded defaults (lowest priority)
	_ = config.Load(confmap.Provider(defaults.Config, "."), nil)

	// Layer 2: .env file (optional, overrides defaults)
	envFilePath := resolveEnvFile()
	if loadErr := config.Load(file.Provider(envFilePath), dotenv.ParserEnv(PREFIX, ".", envKeyTransform)); loadErr == nil {
		logger.Info("loaded .env file", zap.String("path", envFilePath))
	} else if !errors.Is(loadErr, os.ErrNotExist) {
		logger.Warn("failed to load .env file", zap.Error(loadErr))
	}

	// Layer 3: real environment variables (highest priority)
	err = config.Load(env.Provider(PREFIX, ".", envKeyTransform), nil)

	logger.Debug("config keys loaded", zap.Strings("keys", config.Keys()))

	if err != nil {
		return nil, err
	}
	return
}

// resolveEnvFile returns the path to the .env file. It first checks the current
// working directory, then falls back to BUILD_WORKSPACE_DIRECTORY (set by
// `bazel run`) so that .env files at the workspace root are found even when
// the binary runs from the Bazel runfiles sandbox.
func resolveEnvFile() string {
	const envFile = ".env"
	if _, err := os.Stat(envFile); err == nil {
		return envFile
	}
	if wsDir := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); wsDir != "" {
		return filepath.Join(wsDir, envFile)
	}
	return envFile
}
