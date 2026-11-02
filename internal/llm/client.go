package llm

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Options configure a client.
type Options struct {
	// BaseURL is the endpoint. For an OpenAI-compatible API it includes the version path, e.g. "http://tensai:8080/v1",
	// and it is the OpenAI API when empty. For Bedrock it overrides the regional endpoint, e.g. for a VPC endpoint.
	BaseURL string
	Model   string
	APIKey  string
	// Region is the AWS region of Bedrock.
	Region    string
	MaxTokens int
	// Timeout limits each request.
	Timeout time.Duration
	// Retries is how many times a failed request is retried with backoff. A local LLM may still be loading its model.
	Retries int
}

// OpenAI calls an OpenAI-compatible chat completions API.
type OpenAI struct {
	client    openai.Client
	model     string
	maxTokens int
}

// NewOpenAI returns a client for an OpenAI-compatible API.
func NewOpenAI(o Options) *OpenAI {
	opts := []option.RequestOption{option.WithMaxRetries(o.Retries)}
	if o.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(o.BaseURL))
	}
	if o.APIKey != "" {
		opts = append(opts, option.WithAPIKey(o.APIKey))
	}
	if o.Timeout > 0 {
		opts = append(opts, option.WithRequestTimeout(o.Timeout))
	}
	return &OpenAI{client: openai.NewClient(opts...), model: o.Model, maxTokens: o.MaxTokens}
}

// Reasoning models such as Qwen3 may put their thinking before the answer.
var thinkBlock = regexp.MustCompile(`(?s)<think>.*?</think>`)

// cleanReply removes the thinking of a reasoning model, and fails on an empty reply.
func cleanReply(s string) (string, error) {
	out := strings.TrimSpace(thinkBlock.ReplaceAllString(s, ""))
	if out == "" {
		return "", errors.New("the LLM returned an empty reply")
	}
	return out, nil
}

// Complete sends a system prompt and a user message and returns the reply.
func (c *OpenAI) Complete(ctx context.Context, system, user string) (string, error) {
	params := openai.ChatCompletionNewParams{
		Model: c.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(system),
			openai.UserMessage(user),
		},
	}
	if c.maxTokens > 0 {
		// max_tokens rather than max_completion_tokens, because local servers such as tensai only accept this one.
		params.MaxTokens = openai.Int(int64(c.maxTokens))
	}
	res, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("openai.ChatCompletionService#New(): %w", err)
	}
	if len(res.Choices) == 0 {
		return "", errors.New("chat completions returned no choices")
	}
	return cleanReply(res.Choices[0].Message.Content)
}
