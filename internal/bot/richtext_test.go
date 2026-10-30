package bot

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kenchan0130/slack-ai-commentator/internal/rule"
	"github.com/slack-go/slack"
)

// richText is the rich text of a prompt in which the user mentioned a user and a user group, as Slack sends it.
const richText = `{"type":"rich_text","elements":[
	{"type":"rich_text_section","elements":[
		{"type":"text","text":"When the risk is high, mention "},
		{"type":"usergroup","usergroup_id":"S0PR"},
		{"type":"text","text":" and "},
		{"type":"user","user_id":"U0LEAD"},
		{"type":"text","text":" "},
		{"type":"emoji","name":"warning"},
		{"type":"text","text":" with "},
		{"type":"link","url":"https://example.com/guide","text":"the guide"},
		{"type":"text","text":".\n"}
	]},
	{"type":"rich_text_list","style":"ordered","indent":0,"elements":[
		{"type":"rich_text_section","elements":[{"type":"text","text":"Relation"}]},
		{"type":"rich_text_section","elements":[{"type":"text","text":"Mention "},{"type":"user","user_id":"U0LEAD"}]}
	]},
	{"type":"rich_text_quote","elements":[{"type":"text","text":"Be brief"}]}
]}`

func parseRichText(t *testing.T, raw string) slack.RichTextBlock {
	t.Helper()
	var rt slack.RichTextBlock
	if err := json.Unmarshal([]byte(raw), &rt); err != nil {
		t.Fatal(err)
	}
	return rt
}

func TestPromptFromRichText(t *testing.T) {
	prompt, mentions := promptFromRichText(parseRichText(t, richText))
	want := "When the risk is high, mention <!subteam^S0PR> and <@U0LEAD> :warning: with the guide (https://example.com/guide).\n" +
		"1. Relation\n2. Mention <@U0LEAD>\n> Be brief"
	if prompt != want {
		t.Errorf("prompt =\n%s\nwant\n%s", prompt, want)
	}
	if !slices.Equal(mentions, []string{"<!subteam^S0PR>", "<@U0LEAD>"}) {
		t.Errorf("mentions = %q", mentions)
	}
}

func TestSystemPromptPreservesSavedBold(t *testing.T) {
	const input = `{"type":"rich_text","elements":[{"type":"rich_text_section","elements":[{"type":"text","text":"関係性","style":{"bold":true}},{"type":"text","text":": 高"}]}]}`
	r := rule.Rule{Prompt: "関係性: 高", PromptRichText: json.RawMessage(input)}
	if got := systemPrompt(r); !strings.Contains(got, "**関係性**: 高") {
		t.Errorf("systemPrompt() = %q, want bold preserved", got)
	}
}

func TestEscapeReply(t *testing.T) {
	out := escapeReply("Risk! <!subteam^S0PR> <@U0LEAD> <@U0OTHER> <!subteam^S0ALL> <!channel> & more", []string{"<!subteam^S0PR>", "<@U0LEAD>"})
	want := "Risk! <!subteam^S0PR> <@U0LEAD> &lt;@U0OTHER&gt; &lt;!subteam^S0ALL&gt; &lt;!channel&gt; &amp; more"
	if out != want {
		t.Errorf("escapeReply() =\n%s\nwant\n%s", out, want)
	}
}

func TestReplyMentionsOnlyThePromptsMentions(t *testing.T) {
	rt := parseRichText(t, richText)
	prompt, mentions := promptFromRichText(rt)
	r := rule.Rule{ID: "r1", ChannelID: "C1", Trigger: rule.TriggerMessage, Prompt: prompt, Mentions: mentions}
	b, api, l := newTestBot(t, r)
	l.out = "**High risk.** <!subteam^S0PR> please check. Also <@U0SPAM>."
	if err := b.HandleEvent(context.Background(), event(`{"type":"message","channel":"C1","ts":"1.0","user":"U1","text":"the app is down"}`)); err != nil {
		t.Fatal(err)
	}
	drain(t, b)

	if !strings.Contains(l.calls[0], "write the mention exactly as it is") {
		t.Errorf("the LLM was not told how to mention: %q", l.calls[0])
	}
	if got := api.posts[0].values.Get("text"); got != "**High risk.** <!subteam^S0PR> please check. Also &lt;@U0SPAM&gt;." {
		t.Errorf("text = %q", got)
	}
}

func TestRuleKeepsTheRichPrompt(t *testing.T) {
	b, _, _ := newTestBot(t)
	view := modalView(`{"channel_id":"C1"}`, "mention", "", "", "")
	view.State.Values[blockPrompt] = map[string]slack.BlockAction{actionInput: {RichTextValue: parseRichText(t, richText)}}
	r, errs, err := b.parseRuleModal(view, "U1", time.Unix(100, 0))
	if err != nil || errs != nil {
		t.Fatalf("parseRuleModal() = %v, %v", errs, err)
	}
	if len(r.Mentions) != 2 || !strings.Contains(r.Prompt, "<!subteam^S0PR>") {
		t.Errorf("rule = %+v", r)
	}

	// The modal to edit the rule shows the rich text as it was written, and the list shows it too.
	modal := b.ruleModal("C1", &r)
	for _, blk := range modal.Blocks.BlockSet {
		if in, ok := blk.(*slack.InputBlock); ok && in.BlockID == blockPrompt {
			if v := in.Element.(*slack.RichTextInputBlockElement).InitialValue; v == nil || len(v.Elements) != 3 {
				t.Errorf("initial value = %+v", v)
			}
		}
	}
	if blocks := b.describe(r); len(blocks) != 2 || len(blocks[1].(*slack.RichTextBlock).Elements) != 3 {
		t.Errorf("describe() = %+v", blocks)
	}
}
