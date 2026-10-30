package bot

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kenchan0130/slack-ai-commentator/internal/llm"
	"github.com/kenchan0130/slack-ai-commentator/internal/rule"
)

// TestReplyIntegration builds the prompt the way the app does and calls a real LLM, e.g. LLM_TEST_URL=http://localhost:8080/v1.
func TestReplyIntegration(t *testing.T) {
	url := os.Getenv("LLM_TEST_URL")
	if url == "" {
		t.Skip("LLM_TEST_URL is not set")
	}
	r := rule.Rule{ID: "r1", ChannelID: "C1", Trigger: rule.TriggerMessage, Authors: []string{"RSS"},
		Prompt: "あなたは企業の評判モニタリング担当です。投稿が「サンプルバーガー」にどう関係するか、感情（ポジティブ・中立・ネガティブ）、風評リスクの有無を、日本語で3行以内で答えてください。"}
	b, api, _ := newTestBot(t, r)
	b.llm = llm.NewOpenAI(llm.Options{BaseURL: url, Model: os.Getenv("LLM_TEST_MODEL"), APIKey: os.Getenv("LLM_TEST_API_KEY"), MaxTokens: 300, Timeout: 5 * time.Minute})

	ev := `{"type":"message","subtype":"bot_message","channel":"C1","ts":"1.0","bot_id":"B1","bot_profile":{"name":"RSS"},
		"attachments":[{"title":"サンプルバーガーの駅前店、店員の対応がひどかった。もう行かない。","title_link":"https://x.com/a/status/1"}]}`
	if err := b.HandleEvent(context.Background(), event(ev)); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	drain(t, b)
	if len(api.posts) != 1 {
		t.Fatalf("posts = %+v", api.posts)
	}
	t.Logf("%s:\n%s", time.Since(start), api.posts[0].values.Get("text"))
}
