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

const (
	envPrefix = "gpt"

	keyOpenAIAPIKey               = "openai_api_key"
	keyServiceSecret              = "service_secret"
	keyLogLevel                   = "log_level"
	keySystemPrompt               = "system_prompt"
	keyWorkers                    = "workers"
	keyQueueSize                  = "queue_size"
	keyPort                       = "port"
	keyRequestTimeoutSeconds      = "request_timeout_seconds"
	keyUpstreamPollTimeoutSeconds = "upstream_poll_timeout_seconds"
	keyMaxOutputTokens            = "max_output_tokens"

	flagOpenAIAPIKey        = keyOpenAIAPIKey
	flagServiceSecret       = keyServiceSecret
	flagLogLevel            = keyLogLevel
	flagSystemPrompt        = keySystemPrompt
	flagWorkers             = keyWorkers
	flagQueueSize           = keyQueueSize
	flagPort                = keyPort
	flagRequestTimeout      = "request_timeout"
	flagUpstreamPollTimeout = "upstream_poll_timeout"
	flagMaxOutputTokens     = keyMaxOutputTokens

	envOpenAIAPIKey               = "OPENAI_API_KEY"
	envServiceSecret              = "SERVICE_SECRET"
	envLogLevel                   = "LOG_LEVEL"
	envSystemPrompt               = "SYSTEM_PROMPT"
	envWorkers                    = "GPT_WORKERS"
	envQueueSize                  = "GPT_QUEUE_SIZE"
	envPort                       = "HTTP_PORT"
	envRequestTimeoutSeconds      = "GPT_REQUEST_TIMEOUT_SECONDS"
	envUpstreamPollTimeoutSeconds = "GPT_UPSTREAM_POLL_TIMEOUT_SECONDS"
	envMaxOutputTokens            = "GPT_MAX_OUTPUT_TOKENS"

	quoteCharacters = "\"'"
	logLevelDebug   = "debug"
	logLevelInfo    = "info"
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
		if !cmd.Flags().Changed(flagServiceSecret) {
			config.ServiceSecret = strings.TrimSpace(strings.Trim(viper.GetString(keyServiceSecret), quoteCharacters))
		}
		if !cmd.Flags().Changed(flagOpenAIAPIKey) {
			config.OpenAIKey = strings.TrimSpace(strings.Trim(viper.GetString(keyOpenAIAPIKey), quoteCharacters))
		}
		if !cmd.Flags().Changed(flagPort) {
			config.Port = viper.GetInt(keyPort)
		}
		if config.Port == 0 {
			config.Port = proxy.DefaultPort
		}
		if !cmd.Flags().Changed(flagLogLevel) {
			config.LogLevel = viper.GetString(keyLogLevel)
		}
		if config.LogLevel == "" {
			config.LogLevel = logLevelInfo
		}
		if !cmd.Flags().Changed(flagSystemPrompt) {
			config.SystemPrompt = viper.GetString(keySystemPrompt)
		}
		if !cmd.Flags().Changed(flagWorkers) {
			config.WorkerCount = viper.GetInt(keyWorkers)
		}
		if config.WorkerCount == 0 {
			config.WorkerCount = proxy.DefaultWorkers
		}
		if !cmd.Flags().Changed(flagQueueSize) {
			config.QueueSize = viper.GetInt(keyQueueSize)
		}
		if config.QueueSize == 0 {
			config.QueueSize = proxy.DefaultQueueSize
		}
		if !cmd.Flags().Changed(flagRequestTimeout) {
			config.RequestTimeoutSeconds = viper.GetInt(keyRequestTimeoutSeconds)
		}
		if config.RequestTimeoutSeconds == 0 {
			config.RequestTimeoutSeconds = proxy.DefaultRequestTimeoutSeconds
		}
		if !cmd.Flags().Changed(flagUpstreamPollTimeout) {
			config.UpstreamPollTimeoutSeconds = viper.GetInt(keyUpstreamPollTimeoutSeconds)
		}
		if config.UpstreamPollTimeoutSeconds == 0 {
			config.UpstreamPollTimeoutSeconds = proxy.DefaultUpstreamPollTimeoutSeconds
		}
		if !cmd.Flags().Changed(flagMaxOutputTokens) {
			config.MaxOutputTokens = viper.GetInt(keyMaxOutputTokens)
		}
		if config.MaxOutputTokens == 0 {
			config.MaxOutputTokens = proxy.DefaultMaxOutputTokens
		}

		var logger *zap.Logger
		var loggerError error
		switch strings.ToLower(config.LogLevel) {
		case logLevelDebug:
			logger, loggerError = zap.NewDevelopment()
		default:
			logger, loggerError = zap.NewProduction()
		}
		if loggerError != nil {
			return loggerError
		}
		defer func() { _ = logger.Sync() }()
		sugar := logger.Sugar()

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
	if err := viper.BindEnv(keyOpenAIAPIKey, envOpenAIAPIKey); err != nil {
		errs = append(errs, keyOpenAIAPIKey+":"+err.Error())
	}
	if err := viper.BindEnv(keyServiceSecret, envServiceSecret); err != nil {
		errs = append(errs, keyServiceSecret+":"+err.Error())
	}
	if err := viper.BindEnv(keyLogLevel, envLogLevel); err != nil {
		errs = append(errs, keyLogLevel+":"+err.Error())
	}
	if err := viper.BindEnv(keySystemPrompt, envSystemPrompt); err != nil {
		errs = append(errs, keySystemPrompt+":"+err.Error())
	}
	if err := viper.BindEnv(keyWorkers, envWorkers); err != nil {
		errs = append(errs, keyWorkers+":"+err.Error())
	}
	if err := viper.BindEnv(keyQueueSize, envQueueSize); err != nil {
		errs = append(errs, keyQueueSize+":"+err.Error())
	}
	if err := viper.BindEnv(keyPort, envPort); err != nil {
		errs = append(errs, keyPort+":"+err.Error())
	}
	if err := viper.BindEnv(keyRequestTimeoutSeconds, envRequestTimeoutSeconds); err != nil {
		errs = append(errs, keyRequestTimeoutSeconds+":"+err.Error())
	}
	if err := viper.BindEnv(keyUpstreamPollTimeoutSeconds, envUpstreamPollTimeoutSeconds); err != nil {
		errs = append(errs, keyUpstreamPollTimeoutSeconds+":"+err.Error())
	}
	if err := viper.BindEnv(keyMaxOutputTokens, envMaxOutputTokens); err != nil {
		errs = append(errs, keyMaxOutputTokens+":"+err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func init() {
	viper.SetEnvPrefix(envPrefix)
	viper.AutomaticEnv()

	if err := bindOrDie(); err != nil {
		panic("viper env binding failed: " + err.Error())
	}

	rootCmd.Flags().StringVar(
		&config.ServiceSecret,
		flagServiceSecret,
		"",
		"shared secret for requests (env: "+envServiceSecret+")",
	)
	rootCmd.Flags().StringVar(
		&config.OpenAIKey,
		flagOpenAIAPIKey,
		"",
		"OpenAI API key (env: "+envOpenAIAPIKey+")",
	)
	rootCmd.Flags().IntVar(
		&config.Port,
		flagPort,
		0,
		"TCP port to listen on (env: "+envPort+")",
	)
	rootCmd.Flags().StringVar(
		&config.LogLevel,
		flagLogLevel,
		"",
		"logging level: debug or info (env: "+envLogLevel+")",
	)
	rootCmd.Flags().StringVar(
		&config.SystemPrompt,
		flagSystemPrompt,
		"",
		"system prompt sent to the model (env: "+envSystemPrompt+")",
	)
	rootCmd.Flags().IntVar(
		&config.WorkerCount,
		flagWorkers,
		0,
		"number of worker goroutines (env: "+envWorkers+")",
	)
	rootCmd.Flags().IntVar(
		&config.QueueSize,
		flagQueueSize,
		0,
		"request queue size (env: "+envQueueSize+")",
	)
	rootCmd.Flags().IntVar(
		&config.RequestTimeoutSeconds,
		flagRequestTimeout,
		0,
		"overall request timeout in seconds (env: "+envRequestTimeoutSeconds+")",
	)
	rootCmd.Flags().IntVar(
		&config.UpstreamPollTimeoutSeconds,
		flagUpstreamPollTimeout,
		0,
		"upstream poll timeout in seconds for incomplete responses (env: "+envUpstreamPollTimeoutSeconds+")",
	)
	rootCmd.Flags().IntVar(
		&config.MaxOutputTokens,
		flagMaxOutputTokens,
		0,
		"maximum output tokens (env: "+envMaxOutputTokens+")",
	)

	if err := viper.BindPFlags(rootCmd.Flags()); err != nil {
		panic("failed to bind flags: " + err.Error())
	}
}
