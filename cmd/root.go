// Package cmd implements the command-line interface for llm-proxy.
package cmd

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var config Configuration

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
		if config.ServiceSecret == "" {
			config.ServiceSecret = viper.GetString("service_secret")
		}
		if config.OpenAIKey == "" {
			config.OpenAIKey = viper.GetString("openai_api_key")
		}
		if config.Port == 0 {
			config.Port = viper.GetInt("port")
		}
		if config.LogLevel == "" {
			config.LogLevel = viper.GetString("log_level")
		}
		if config.SystemPrompt == "" {
			config.SystemPrompt = viper.GetString("system_prompt")
		}

		var logger *zap.Logger
		normalizedLevel := strings.ToLower(config.LogLevel)
		if normalizedLevel == "debug" {
			logger, _ = zap.NewDevelopment()
		} else {
			logger, _ = zap.NewProduction()
		}
		defer logger.Sync()
		sugar := logger.Sugar()

		sugar.Infow("starting proxy", "port", config.Port, "log_level", normalizedLevel)
		return serve(config, sugar)
	},
}

func init() {
	viper.SetEnvPrefix("gpt")
	viper.AutomaticEnv()

	viper.BindEnv("openai_api_key", "OPENAI_API_KEY")
	viper.BindEnv("service_secret", "SERVICE_SECRET")
	viper.BindEnv("log_level", "LOG_LEVEL")
	viper.BindEnv("system_prompt", "SYSTEM_PROMPT")

	rootCmd.Flags().StringVar(
		&config.ServiceSecret,
		"service_secret",
		"",
		"shared secret for requests (env: SERVICE_SECRET)",
	)
	rootCmd.Flags().StringVar(
		&config.OpenAIKey,
		"openai_api_key",
		"",
		"OpenAI API key (env: OPENAI_API_KEY)",
	)
	rootCmd.Flags().IntVar(
		&config.Port,
		"port",
		defaultPort,
		"TCP port to listen on (env: GPT_PORT)",
	)
	rootCmd.Flags().StringVar(
		&config.LogLevel,
		"log_level",
		"info",
		"logging level: debug or info (env: LOG_LEVEL)",
	)
	rootCmd.Flags().StringVar(
		&config.SystemPrompt,
		"system_prompt",
		"",
		"system prompt sent to the model (env: SYSTEM_PROMPT)",
	)

	viper.BindPFlags(rootCmd.Flags())
}
