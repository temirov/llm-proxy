package main

import (
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/temirov/llm-proxy/internal/apperrors"
	"github.com/temirov/llm-proxy/internal/constants"
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
)

const (
	// messageServiceSecretEmpty is logged when SERVICE_SECRET is missing.
	messageServiceSecretEmpty = "SERVICE_SECRET is empty; refusing to start"
	// messageOpenAIAPIKeyEmpty is logged when OPENAI_API_KEY is missing.
	messageOpenAIAPIKeyEmpty = "OPENAI_API_KEY is empty; refusing to start"
	// logEventStartingProxy indicates the proxy is starting.
	logEventStartingProxy = "starting proxy"
)

const (
	// bindingErrorSeparator is used to join multiple binding errors.
	bindingErrorSeparator = "; "
)

var config proxy.Configuration

const (
	// rootCmdShort provides a brief description of the root command.
	// Additional commands should define their short description using a constant following this pattern.
	rootCmdShort = "Tiny HTTP proxy for ChatGPT"

	// rootCmdLong provides a detailed description of the root command.
	// Additional commands should define their long description using a constant following this pattern.
	rootCmdLong = "Accepts GET /?prompt=…&key=SECRET and forwards to OpenAI."

	// rootCmdExample demonstrates how to use the root command.
	// Additional commands should define their usage examples using a constant following this pattern.
	rootCmdExample = `llm-proxy --service_secret=mysecret --openai_api_key=sk-xxxxx --log_level=debug
SERVICE_SECRET=mysecret OPENAI_API_KEY=sk-xxxxx LOG_LEVEL=debug llm-proxy`
)

// Execute runs the command-line interface.
func Execute() {
	rootCmd.SilenceUsage = false
	rootCmd.SilenceErrors = false
	if executeError := rootCmd.Execute(); executeError != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:     "llm-proxy",
	Short:   rootCmdShort,
	Long:    rootCmdLong,
	Example: rootCmdExample,
	RunE: func(command *cobra.Command, arguments []string) error {
		populateStringConfiguration(command, flagServiceSecret, keyServiceSecret, &config.ServiceSecret, constants.EmptyString, trimSpacesAndQuotes)
		populateStringConfiguration(command, flagOpenAIAPIKey, keyOpenAIAPIKey, &config.OpenAIKey, constants.EmptyString, trimSpacesAndQuotes)
		populateIntConfiguration(command, flagPort, keyPort, &config.Port, proxy.DefaultPort)
		populateStringConfiguration(command, flagLogLevel, keyLogLevel, &config.LogLevel, proxy.LogLevelInfo, identityTransformer)
		populateStringConfiguration(command, flagSystemPrompt, keySystemPrompt, &config.SystemPrompt, constants.EmptyString, identityTransformer)
		populateIntConfiguration(command, flagWorkers, keyWorkers, &config.WorkerCount, proxy.DefaultWorkers)
		populateIntConfiguration(command, flagQueueSize, keyQueueSize, &config.QueueSize, proxy.DefaultQueueSize)
		populateIntConfiguration(command, flagRequestTimeout, keyRequestTimeoutSeconds, &config.RequestTimeoutSeconds, proxy.DefaultRequestTimeoutSeconds)
		populateIntConfiguration(command, flagUpstreamPollTimeout, keyUpstreamPollTimeoutSeconds, &config.UpstreamPollTimeoutSeconds, proxy.DefaultUpstreamPollTimeoutSeconds)
		populateIntConfiguration(command, flagMaxOutputTokens, keyMaxOutputTokens, &config.MaxOutputTokens, proxy.DefaultMaxOutputTokens)

		var logger *zap.Logger
		var loggerError error
		switch strings.ToLower(config.LogLevel) {
		case proxy.LogLevelDebug:
			logger, loggerError = zap.NewDevelopment()
		default:
			logger, loggerError = zap.NewProduction()
		}
		if loggerError != nil {
			return loggerError
		}
		defer func() { _ = logger.Sync() }()
		sugar := logger.Sugar()

		if strings.TrimSpace(config.ServiceSecret) == constants.EmptyString {
			sugar.Error(messageServiceSecretEmpty)
			return apperrors.ErrMissingServiceSecret
		}
		if strings.TrimSpace(config.OpenAIKey) == constants.EmptyString {
			sugar.Error(messageOpenAIAPIKeyEmpty)
			return apperrors.ErrMissingOpenAIKey
		}

		sugar.Infow(logEventStartingProxy,
			"port", config.Port,
			"log_level", strings.ToLower(config.LogLevel),
			"secret_fingerprint", utils.Fingerprint(config.ServiceSecret),
		)
		return proxy.Serve(config, sugar)
	},
}

// bindOrDie wraps viper bindings and returns a combined error if any bind fails.
func bindOrDie() error {
	var bindingErrors []string
	if bindError := viper.BindEnv(keyOpenAIAPIKey, envOpenAIAPIKey); bindError != nil {
		bindingErrors = append(bindingErrors, keyOpenAIAPIKey+":"+bindError.Error())
	}
	if bindError := viper.BindEnv(keyServiceSecret, envServiceSecret); bindError != nil {
		bindingErrors = append(bindingErrors, keyServiceSecret+":"+bindError.Error())
	}
	if bindError := viper.BindEnv(keyLogLevel, envLogLevel); bindError != nil {
		bindingErrors = append(bindingErrors, keyLogLevel+":"+bindError.Error())
	}
	if bindError := viper.BindEnv(keySystemPrompt, envSystemPrompt); bindError != nil {
		bindingErrors = append(bindingErrors, keySystemPrompt+":"+bindError.Error())
	}
	if bindError := viper.BindEnv(keyWorkers, envWorkers); bindError != nil {
		bindingErrors = append(bindingErrors, keyWorkers+":"+bindError.Error())
	}
	if bindError := viper.BindEnv(keyQueueSize, envQueueSize); bindError != nil {
		bindingErrors = append(bindingErrors, keyQueueSize+":"+bindError.Error())
	}
	if bindError := viper.BindEnv(keyPort, envPort); bindError != nil {
		bindingErrors = append(bindingErrors, keyPort+":"+bindError.Error())
	}
	if bindError := viper.BindEnv(keyRequestTimeoutSeconds, envRequestTimeoutSeconds); bindError != nil {
		bindingErrors = append(bindingErrors, keyRequestTimeoutSeconds+":"+bindError.Error())
	}
	if bindError := viper.BindEnv(keyUpstreamPollTimeoutSeconds, envUpstreamPollTimeoutSeconds); bindError != nil {
		bindingErrors = append(bindingErrors, keyUpstreamPollTimeoutSeconds+":"+bindError.Error())
	}
	if bindError := viper.BindEnv(keyMaxOutputTokens, envMaxOutputTokens); bindError != nil {
		bindingErrors = append(bindingErrors, keyMaxOutputTokens+":"+bindError.Error())
	}
	if len(bindingErrors) > 0 {
		return errors.New(strings.Join(bindingErrors, bindingErrorSeparator))
	}
	return nil
}

func init() {
	viper.SetEnvPrefix(envPrefix)
	viper.AutomaticEnv()

	if bindError := bindOrDie(); bindError != nil {
		panic("viper env binding failed: " + bindError.Error())
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

	if flagBindError := viper.BindPFlags(rootCmd.Flags()); flagBindError != nil {
		panic("failed to bind flags: " + flagBindError.Error())
	}
}
