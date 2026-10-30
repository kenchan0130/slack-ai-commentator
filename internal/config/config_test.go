package config

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"
	"time"
)

// base has every required setting for an OpenAI-compatible API.
func base() map[string]string {
	return map[string]string{
		"SLACK_BOT_TOKEN": "xoxb",
		"SLACK_APP_TOKEN": "xapp",
		"UI_LANGUAGE":     "ja",
		"STORE":           "file:///opt/data/rules.json",
		"LLM_PROVIDER":    "openai",
		"LLM_URL":         "http://tensai:8080/v1",
		"LLM_MODEL":       "qwen3",
		"LLM_MAX_TOKENS":  "1024",
		"LLM_CONCURRENCY": "1",
	}
}

func TestLoadTuning(t *testing.T) {
	c, err := Load(context.Background(), base(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.LLMTimeout != 5*time.Minute || c.LLMRetries != 10 || c.QueueSize != 100 || c.MaxInputChars != 4000 {
		t.Errorf("defaults = %+v", c)
	}

	environ := with(with(with(with(base(), "LLM_TIMEOUT", "30s"), "LLM_RETRIES", "0"), "QUEUE_SIZE", "5"), "MAX_INPUT_CHARS", "10")
	if c, err = Load(context.Background(), environ, nil); err != nil {
		t.Fatal(err)
	}
	if c.LLMTimeout != 30*time.Second || c.LLMRetries != 0 || c.QueueSize != 5 || c.MaxInputChars != 10 {
		t.Errorf("tuning = %+v", c)
	}
}

// with returns a copy of m with key set to value, or without key when value is empty.
func with(m map[string]string, key, value string) map[string]string {
	m = maps.Clone(m)
	if value == "" {
		delete(m, key)
	} else {
		m[key] = value
	}
	return m
}

func TestLoad(t *testing.T) {
	c, err := Load(context.Background(), base(), func(context.Context, string) (string, error) {
		t.Fatal("ssm must not be called without ssm:// values")
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.UILanguage != "ja" || c.Store != "file:///opt/data/rules.json" || c.LLMProvider != ProviderOpenAI || c.LLMModel != "qwen3" ||
		c.LLMConcurrency != 1 || c.LLMMaxTokens != 1024 || c.Debug {
		t.Errorf("config = %+v", c)
	}
}

// bedrock has every required setting for Amazon Bedrock.
func bedrock() map[string]string {
	return with(with(with(base(), "LLM_PROVIDER", "bedrock"), "LLM_URL", ""), "AWS_BEDROCK_REGION", "us-east-1")
}

func TestLoadBedrock(t *testing.T) {
	c, err := Load(context.Background(), bedrock(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.LLMProvider != ProviderBedrock || c.LLMURL != "" || c.BedrockRegion != "us-east-1" {
		t.Errorf("config = %+v", c)
	}
}

func TestLoadResolvesSSM(t *testing.T) {
	var asked []string
	environ := with(with(with(base(), "SLACK_BOT_TOKEN", "ssm:///app/bot"), "LLM_API_KEY", "ssm:///app/key"), "AWS_SSM_REGION", "ap-northeast-1")
	c, err := Load(context.Background(), environ,
		func(_ context.Context, name string) (string, error) {
			asked = append(asked, name)
			return "resolved:" + name, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if c.SlackBotToken != "resolved:/app/bot" || c.SlackAppToken != "xapp" || c.LLMAPIKey != "resolved:/app/key" || len(asked) != 2 {
		t.Errorf("asked = %v, config = %+v", asked, c)
	}
}

func TestLoadErrors(t *testing.T) {
	denied := func(context.Context, string) (string, error) { return "", errors.New("access denied") }
	cases := map[string]struct {
		environ map[string]string
		want    string
	}{
		"duration":    {with(base(), "LLM_TIMEOUT", "soon"), `invalid duration "soon"`},
		"concurrency": {with(base(), "LLM_CONCURRENCY", "0"), "LLM_CONCURRENCY"},
		"ssm":         {with(with(base(), "SLACK_APP_TOKEN", "ssm:///missing"), "AWS_SSM_REGION", "ap-northeast-1"), "SLACK_APP_TOKEN: access denied"},
		"ssm region":  {with(base(), "SLACK_APP_TOKEN", "ssm:///x"), "AWS_SSM_REGION is required"},
		"provider":    {with(base(), "LLM_PROVIDER", "azure"), "LLM_PROVIDER"},
		"url":         {with(base(), "LLM_URL", ""), "LLM_URL is required"},
		// AWS_REGION is not the region of Bedrock.
		"bedrock region": {with(with(bedrock(), "AWS_BEDROCK_REGION", ""), "AWS_REGION", "us-east-1"), "AWS_BEDROCK_REGION is required"},
	}
	// Every setting in base has to be given.
	for name := range base() {
		if name != "LLM_URL" {
			cases["missing "+name] = struct {
				environ map[string]string
				want    string
			}{with(base(), name, ""), name}
		}
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(context.Background(), tc.environ, denied)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load() error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}
