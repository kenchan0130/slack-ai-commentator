package bot

import (
	"context"
	"testing"

	"github.com/kenchan0130/slack-ai-commentator/internal/rule"
)

func TestSameEmoji(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"+1", "+1", true},
		{"+1", "thumbsup", true},
		{"thumbs_up", "+1", true},
		{"mag", "mag", true},
		{"+1", "-1", false},
		{"mag", "mag_right", false},
		// Custom emoji are compared by their names.
		{"mag-acme", "mag-acme", true},
		{"mag-acme", "mag", false},
		{"no-such-emoji", "another-unknown", false},
	} {
		if got := sameEmoji(tc.a, tc.b); got != tc.want {
			t.Errorf("sameEmoji(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestReactionMatchesAnAlias(t *testing.T) {
	// The rule stores the alias chosen in the modal, and Slack sends another name with the reaction.
	b, _, _ := newTestBot(t, rule.Rule{ID: "r1", ChannelID: "C1", Trigger: rule.TriggerReaction, Emoji: "thumbs_up", Prompt: "p"})
	if err := b.HandleEvent(context.Background(), event(`{"type":"reaction_added","user":"U2","reaction":"+1::skin-tone-2","item":{"type":"message","channel":"C1","ts":"1.0"}}`)); err != nil {
		t.Fatal(err)
	}
	if n := len(b.jobs); n != 1 {
		t.Errorf("queued %d jobs, want the reaction to match the alias of the rule", n)
	}
}
