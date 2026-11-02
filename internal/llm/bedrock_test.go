package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBedrockComplete(t *testing.T) {
	// AWS_REGION does not change the region of Bedrock.
	t.Setenv("AWS_REGION", "ap-northeast-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_CONFIG_FILE", "/nonexistent")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent")

	var got struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
		InferenceConfig struct {
			MaxTokens int `json:"maxTokens"`
		} `json:"inferenceConfig"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model/my.model-v1/converse" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if a := r.Header.Get("Authorization"); !strings.HasPrefix(a, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/") || !strings.Contains(a, "/us-east-1/bedrock/") {
			t.Errorf("Authorization = %q, want a SigV4 signature for us-east-1", a)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"reasoningContent":{"reasoningText":{"text":"hmm"}}},{"text":"answer"}]}},
			"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2},"metrics":{"latencyMs":1}}`))
	}))
	defer srv.Close()

	c, err := NewBedrock(context.Background(), Options{BaseURL: srv.URL, Model: "my.model-v1", Region: "us-east-1", MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Complete(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatal(err)
	}
	if out != "answer" {
		t.Errorf("Complete() = %q, want only the text", out)
	}
	if len(got.System) != 1 || got.System[0].Text != "sys" || len(got.Messages) != 1 || got.Messages[0].Role != "user" ||
		got.Messages[0].Content[0].Text != "usr" || got.InferenceConfig.MaxTokens != 100 {
		t.Errorf("request = %+v", got)
	}
}

func TestNewBedrockRequiresRegion(t *testing.T) {
	// AWS_REGION is not used for Bedrock.
	t.Setenv("AWS_REGION", "ap-northeast-1")
	if _, err := NewBedrock(context.Background(), Options{Model: "m"}); err == nil || !strings.Contains(err.Error(), "region") {
		t.Fatalf("NewBedrock() error = %v", err)
	}
}
