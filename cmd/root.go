package cmd

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var cfg Configuration

// Execute runs the command-line interface.
func Execute() {
	rootCmd.SilenceUsage = false
	rootCmd.SilenceErrors = false
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "llm-proxy",
	Short: "Tiny HTTP proxy for ChatGPT",
	Long:  "Accepts GET /?prompt=…&key=SECRET and forwards to OpenAI.",
	Example: `llm-proxy --service_secret=mysecret --openai_api_key=sk-xxxxx --log_level=debug
SERVICE_SECRET=mysecret OPENAI_API_KEY=sk-xxxxx LOG_LEVEL=debug llm-proxy`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// fill in from env if flags didn't
		if cfg.ServiceSecret == "" {
			cfg.ServiceSecret = viper.GetString("service_secret")
		}
		if cfg.OpenAIKey == "" {
			cfg.OpenAIKey = viper.GetString("openai_api_key")
		}
		if cfg.Port == 0 {
			cfg.Port = viper.GetInt("port")
		}
		if cfg.LogLevel == "" {
			cfg.LogLevel = viper.GetString("log_level")
		}
		if cfg.SystemPrompt == "" {
			cfg.SystemPrompt = viper.GetString("system_prompt")
		}

		// choose zap config based on log level
		var logger *zap.Logger
		lvl := strings.ToLower(cfg.LogLevel)
		if lvl == "debug" {
			logger, _ = zap.NewDevelopment()
		} else {
			logger, _ = zap.NewProduction()
		}
		defer logger.Sync()
		sugar := logger.Sugar()

		sugar.Infow("starting proxy", "port", cfg.Port, "log_level", lvl)
		return serve(cfg, sugar)
	},
}

func init() {
	viper.SetEnvPrefix("gpt")
	viper.AutomaticEnv()

	// bind OPENAI_API_KEY, SERVICE_SECRET, LOG_LEVEL from environment
	viper.BindEnv("openai_api_key", "OPENAI_API_KEY")
	viper.BindEnv("service_secret", "SERVICE_SECRET")
	viper.BindEnv("log_level", "LOG_LEVEL")
	viper.BindEnv("system_prompt", "SYSTEM_PROMPT")

	rootCmd.Flags().StringVar(
		&cfg.ServiceSecret,
		"service_secret",
		"",
		"shared secret for requests (env: SERVICE_SECRET)",
	)
	rootCmd.Flags().StringVar(
		&cfg.OpenAIKey,
		"openai_api_key",
		"",
		"OpenAI API key (env: OPENAI_API_KEY)",
	)
	rootCmd.Flags().IntVar(
		&cfg.Port,
		"port",
		defaultPort,
		"TCP port to listen on (env: GPT_PORT)",
	)
	rootCmd.Flags().StringVar(
		&cfg.LogLevel,
		"log_level",
		"info",
		"logging level: debug or info (env: LOG_LEVEL)",
	)
	rootCmd.Flags().StringVar(
		&cfg.SystemPrompt,
		"system_prompt",
		"",
		"system prompt sent to the model (env: SYSTEM_PROMPT)",
	)

	viper.BindPFlags(rootCmd.Flags())
}
