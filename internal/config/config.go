package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/caarlos0/env/v11"
)

// Config is read from environment variables. The settings that depend on the environment or the LLM are given explicitly.
// Only the tuning that suits any environment has a default.
type Config struct {
	SlackBotToken string `env:"SLACK_BOT_TOKEN,required,notEmpty"`
	SlackAppToken string `env:"SLACK_APP_TOKEN,required,notEmpty"`
	Debug         bool   `env:"DEBUG"`
	// UILanguage is the language of the messages in Slack. LANGUAGE is not used, because gettext uses it for a list such as "ja:en".
	UILanguage string `env:"UI_LANGUAGE,required,notEmpty"`

	Store string `env:"STORE,required,notEmpty"`

	// SSMRegion is the region of the SSM parameters of the "ssm://" secrets. It is required when one is used.
	SSMRegion string `env:"AWS_SSM_REGION"`

	// LLMProvider is "openai" for an OpenAI-compatible API, or "bedrock" for Amazon Bedrock with the default AWS credentials.
	LLMProvider string `env:"LLM_PROVIDER,required,notEmpty"`
	// LLMURL is required for an OpenAI-compatible API. For Bedrock, it optionally overrides the endpoint.
	LLMURL    string `env:"LLM_URL"`
	LLMModel  string `env:"LLM_MODEL,required,notEmpty"`
	LLMAPIKey string `env:"LLM_API_KEY"`
	// BedrockRegion is the region of Bedrock, which can differ from SSMRegion, because models often come to some regions first.
	BedrockRegion  string        `env:"AWS_BEDROCK_REGION"`
	LLMMaxTokens   int           `env:"LLM_MAX_TOKENS,required,notEmpty"`
	LLMTimeout     time.Duration `env:"LLM_TIMEOUT" envDefault:"5m"`
	LLMRetries     int           `env:"LLM_RETRIES" envDefault:"10"`
	LLMConcurrency int           `env:"LLM_CONCURRENCY,required,notEmpty"`

	QueueSize     int `env:"QUEUE_SIZE" envDefault:"100"`
	MaxInputChars int `env:"MAX_INPUT_CHARS" envDefault:"4000"`
}

const ssmPrefix = "ssm://"

// LLM providers.
const (
	ProviderOpenAI  = "openai"
	ProviderBedrock = "bedrock"
)

// ParameterGetter resolves a parameter, such as a value in AWS Systems Manager Parameter Store.
type ParameterGetter func(ctx context.Context, name string) (string, error)

// Load reads the configuration from environ. A secret whose value starts with "ssm://", e.g. "ssm:///app/slack_bot_token",
// is read from AWS Systems Manager Parameter Store in AWS_SSM_REGION. A nil getter uses the default AWS credentials.
func Load(ctx context.Context, environ map[string]string, getParameter ParameterGetter) (*Config, error) {
	var c Config
	if err := env.ParseWithOptions(&c, env.Options{Environment: environ}); err != nil {
		return nil, fmt.Errorf("env.ParseWithOptions(): %w", err)
	}
	if c.LLMConcurrency < 1 || c.QueueSize < 1 {
		return nil, errors.New("LLM_CONCURRENCY and QUEUE_SIZE must be at least 1")
	}
	switch c.LLMProvider {
	case ProviderOpenAI:
		if c.LLMURL == "" {
			return nil, errors.New("LLM_URL is required for an OpenAI-compatible API, e.g. https://api.openai.com/v1")
		}
	case ProviderBedrock:
		if c.BedrockRegion == "" {
			return nil, errors.New("AWS_BEDROCK_REGION is required for Amazon Bedrock, e.g. us-east-1")
		}
	default:
		return nil, fmt.Errorf("LLM_PROVIDER must be %q or %q: %q", ProviderOpenAI, ProviderBedrock, c.LLMProvider)
	}

	var errs []error
	for name, secret := range map[string]*string{
		"SLACK_BOT_TOKEN": &c.SlackBotToken,
		"SLACK_APP_TOKEN": &c.SlackAppToken,
		"LLM_API_KEY":     &c.LLMAPIKey,
	} {
		param, ok := strings.CutPrefix(*secret, ssmPrefix)
		if !ok {
			continue
		}
		if c.SSMRegion == "" {
			errs = append(errs, fmt.Errorf("%s: AWS_SSM_REGION is required to read %s", name, *secret))
			continue
		}
		if getParameter == nil {
			getParameter = ssmGetter(c.SSMRegion)
		}
		v, err := getParameter(ctx, param)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		*secret = v
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return &c, nil
}

// LoadFromEnv reads the configuration from the process environment.
func LoadFromEnv(ctx context.Context) (*Config, error) {
	return Load(ctx, env.ToMap(os.Environ()), nil)
}

func ssmGetter(region string) ParameterGetter {
	var client *ssm.Client
	return func(ctx context.Context, name string) (string, error) {
		if client == nil {
			cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
			if err != nil {
				return "", fmt.Errorf("awsconfig.LoadDefaultConfig(): %w", err)
			}
			client = ssm.NewFromConfig(cfg)
		}
		out, err := client.GetParameter(ctx, &ssm.GetParameterInput{Name: &name, WithDecryption: new(true)})
		if err != nil {
			return "", fmt.Errorf("ssm.Client#GetParameter(%s): %w", name, err)
		}
		if out.Parameter == nil || out.Parameter.Value == nil {
			return "", fmt.Errorf("ssm parameter %s has no value", name)
		}
		return *out.Parameter.Value, nil
	}
}
