package llm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// Bedrock calls the Converse API of Amazon Bedrock.
// It signs requests with the default AWS credentials, such as the IAM role of an EC2 instance, so no API key is needed.
type Bedrock struct {
	client    *bedrockruntime.Client
	model     string
	maxTokens int
}

// NewBedrock returns a client for Amazon Bedrock in o.Region.
// The region is given apart from the other AWS services, because models often come to some regions first.
func NewBedrock(ctx context.Context, o Options) (*Bedrock, error) {
	if o.Region == "" {
		return nil, errors.New("the region of Amazon Bedrock is required")
	}
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(o.Region),
		awsconfig.WithRetryMaxAttempts(o.Retries + 1),
	}
	if o.Timeout > 0 {
		// The timeout of the HTTP client limits each attempt, as for an OpenAI-compatible API, and leaves time for retries.
		opts = append(opts, awsconfig.WithHTTPClient(awshttp.NewBuildableClient().WithTimeout(o.Timeout)))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("awsconfig.LoadDefaultConfig(): %w", err)
	}
	client := bedrockruntime.NewFromConfig(cfg, func(bo *bedrockruntime.Options) {
		if o.BaseURL != "" {
			bo.BaseEndpoint = aws.String(o.BaseURL)
		}
	})
	return &Bedrock{client: client, model: o.Model, maxTokens: o.MaxTokens}, nil
}

// Complete sends a system prompt and a user message and returns the reply.
func (b *Bedrock) Complete(ctx context.Context, system, user string) (string, error) {
	in := &bedrockruntime.ConverseInput{
		ModelId: aws.String(b.model),
		System:  []types.SystemContentBlock{&types.SystemContentBlockMemberText{Value: system}},
		Messages: []types.Message{{
			Role:    types.ConversationRoleUser,
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: user}},
		}},
	}
	if b.maxTokens > 0 {
		in.InferenceConfig = &types.InferenceConfiguration{MaxTokens: aws.Int32(int32(min(b.maxTokens, math.MaxInt32)))}
	}
	out, err := b.client.Converse(ctx, in)
	if err != nil {
		return "", fmt.Errorf("bedrockruntime.Client#Converse(): %w", err)
	}
	msg, ok := out.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return "", fmt.Errorf("bedrock returned no message (stop reason %s)", out.StopReason)
	}
	// Reasoning content is in its own blocks, so only the text is the reply.
	var text strings.Builder
	for _, c := range msg.Value.Content {
		if t, ok := c.(*types.ContentBlockMemberText); ok {
			text.WriteString(t.Value)
		}
	}
	return cleanReply(text.String())
}
