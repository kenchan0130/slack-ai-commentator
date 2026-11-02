package llm

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestCompleteIntegration calls a real server, e.g. LLM_TEST_URL=http://localhost:8080/v1 for tensai.
func TestCompleteIntegration(t *testing.T) {
	url := os.Getenv("LLM_TEST_URL")
	if url == "" {
		t.Skip("LLM_TEST_URL is not set")
	}
	c := NewOpenAI(Options{BaseURL: url, Model: os.Getenv("LLM_TEST_MODEL"), APIKey: os.Getenv("LLM_TEST_API_KEY"), MaxTokens: 200, Timeout: 5 * time.Minute})
	start := time.Now()
	out, err := c.Complete(context.Background(),
		"この投稿の内容を一言で要約し、ポジティブ・ネガティブ・中立のどれかを答えてください。",
		"<post>\nサンプルバーガーの新作、パンが固くて食べにくかった。\n</post>")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %s", time.Since(start), out)
}
