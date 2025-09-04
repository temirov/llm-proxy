package main

import (
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/temirov/llm-proxy/internal/apperrors"
	"github.com/temirov/llm-proxy/internal/proxy"
	"github.com/temirov/llm-proxy/internal/utils"
	"go.uber.org/zap"
)

var config proxy.Configuration

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
		// Pull from env if flags were omitted
		if config.ServiceSecret == "" {
			config.ServiceSecret = strings.TrimSpace(strings.Trim(viper.GetString("service_secret"), `"'`))
		}
		if config.OpenAIKey == "" {
			config.OpenAIKey = strings.TrimSpace(strings.Trim(viper.GetString("openai_api_key"), `"'`))
		}
		if config.Port == 0 {
			config.Port = viper.GetInt("port")
			if config.Port == 0 {
				config.Port = proxy.DefaultPort
			}
		}
		if config.LogLevel == "" {
			config.LogLevel = viper.GetString("log_level")
		}
		if config.SystemPrompt == "" {
			config.SystemPrompt = viper.GetString("system_prompt")
		}
		if config.WorkerCount == 0 {
			config.WorkerCount = viper.GetInt("workers")
			if config.WorkerCount == 0 {
				config.WorkerCount = proxy.DefaultWorkers
			}
		}
		if config.QueueSize == 0 {
			config.QueueSize = viper.GetInt("queue_size")
			if config.QueueSize == 0 {
				config.QueueSize = proxy.DefaultQueueSize
			}
		}
		if config.RequestTimeoutSeconds == 0 {
			config.RequestTimeoutSeconds = viper.GetInt("request_timeout_seconds")
			if config.RequestTimeoutSeconds == 0 {
				config.RequestTimeoutSeconds = proxy.DefaultRequestTimeoutSeconds
			}
		}
		if config.UpstreamPollTimeoutSeconds == 0 {
			config.UpstreamPollTimeoutSeconds = viper.GetInt("upstream_poll_timeout_seconds")
			if config.UpstreamPollTimeoutSeconds == 0 {
				config.UpstreamPollTimeoutSeconds = proxy.DefaultUpstreamPollTimeoutSeconds
			}
		}
		if config.MaxOutputTokens == 0 {
			config.MaxOutputTokens = viper.GetInt("max_output_tokens")
			if config.MaxOutputTokens == 0 {
				config.MaxOutputTokens = proxy.DefaultMaxOutputTokens
			}
		}

		var logger *zap.Logger
		var err error
		switch strings.ToLower(config.LogLevel) {
		case "debug":
			logger, err = zap.NewDevelopment()
		default:
			logger, err = zap.NewProduction()
		}
		if err != nil {
			// If logging cannot initialize, there's no sensible way to continue.
			return err
		}
		defer func() { _ = logger.Sync() }()
		sugar := logger.Sugar()

		// Fail fast if secret/key are missing
		if strings.TrimSpace(config.ServiceSecret) == "" {
			sugar.Error("SERVICE_SECRET is empty; refusing to start")
			return apperrors.ErrMissingServiceSecret
		}
		if strings.TrimSpace(config.OpenAIKey) == "" {
			sugar.Error("OPENAI_API_KEY is empty; refusing to start")
			return apperrors.ErrMissingOpenAIKey
		}

		sugar.Infow("starting proxy",
			"port", config.Port,
			"log_level", strings.ToLower(config.LogLevel),
			"secret_fingerprint", utils.Fingerprint(config.ServiceSecret),
		)
		return proxy.Serve(config, sugar)
	},
}

// bindOrDie wraps viper bindings and returns a combined error if any bind fails.
func bindOrDie() error {
	var errs []string
	if err := viper.BindEnv("openai_api_key", "OPENAI_API_KEY"); err != nil {
		errs = append(errs, "openai_api_key:"+err.Error())
	}
	if err := viper.BindEnv("service_secret", "SERVICE_SECRET"); err != nil {
		errs = append(errs, "service_secret:"+err.Error())
	}
	if err := viper.BindEnv("log_level", "LOG_LEVEL"); err != nil {
		errs = append(errs, "log_level:"+err.Error())
	}
	if err := viper.BindEnv("system_prompt", "SYSTEM_PROMPT"); err != nil {
		errs = append(errs, "system_prompt:"+err.Error())
	}
	if err := viper.BindEnv("workers", "GPT_WORKERS"); err != nil {
		errs = append(errs, "workers:"+err.Error())
	}
	if err := viper.BindEnv("queue_size", "GPT_QUEUE_SIZE"); err != nil {
		errs = append(errs, "queue_size:"+err.Error())
	}
	if err := viper.BindEnv("port", "HTTP_PORT"); err != nil {
		errs = append(errs, "port:"+err.Error())
	}
	if err := viper.BindEnv("request_timeout_seconds", "GPT_REQUEST_TIMEOUT_SECONDS"); err != nil {
		errs = append(errs, "request_timeout_seconds:"+err.Error())
	}
	if err := viper.BindEnv("upstream_poll_timeout_seconds", "GPT_UPSTREAM_POLL_TIMEOUT_SECONDS"); err != nil {
		errs = append(errs, "upstream_poll_timeout_seconds:"+err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func init() {
	viper.SetEnvPrefix("gpt")
	viper.AutomaticEnv()

	if err := bindOrDie(); err != nil {
		panic("viper env binding failed: " + err.Error())
	}

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
		proxy.DefaultPort,
		"TCP port to listen on (env: HTTP_PORT)",
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
	rootCmd.Flags().IntVar(
		&config.WorkerCount,
		"workers",
		proxy.DefaultWorkers,
		"number of worker goroutines (env: GPT_WORKERS)",
	)
	rootCmd.Flags().IntVar(
		&config.QueueSize,
		"queue_size",
		proxy.DefaultQueueSize,
		"request queue size (env: GPT_QUEUE_SIZE)",
	)
	rootCmd.Flags().IntVar(
		&config.RequestTimeoutSeconds,
		"request_timeout",
		proxy.DefaultRequestTimeoutSeconds,
		"overall request timeout in seconds (env: GPT_REQUEST_TIMEOUT_SECONDS)",
	)
	rootCmd.Flags().IntVar(
		&config.UpstreamPollTimeoutSeconds,
		"upstream_poll_timeout",
		proxy.DefaultUpstreamPollTimeoutSeconds,
		"upstream poll timeout in seconds for incomplete responses (env: GPT_UPSTREAM_POLL_TIMEOUT_SECONDS)",
	)

	if err := viper.BindPFlags(rootCmd.Flags()); err != nil {
		panic("failed to bind flags: " + err.Error())
	}
}
