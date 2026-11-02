package rule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"uuid"
)

// Trigger is what makes a rule reply to a post.
type Trigger string

const (
	// TriggerMention replies when the app is mentioned.
	TriggerMention Trigger = "mention"
	// TriggerMessage replies to every top-level post in the channel.
	TriggerMessage Trigger = "message"
	// TriggerReaction replies when a post gets the rule's emoji reaction.
	TriggerReaction Trigger = "reaction"
)

// Triggers lists the triggers in the order they are shown to users.
var Triggers = []Trigger{TriggerMention, TriggerMessage, TriggerReaction}

// Valid reports whether t is a known trigger.
func (t Trigger) Valid() bool {
	return slices.Contains(Triggers, t)
}

// Rule is a fixed prompt that replies in the thread of the posts it matches.
type Rule struct {
	ID        string  `json:"id"`
	ChannelID string  `json:"channel_id"`
	Trigger   Trigger `json:"trigger"`
	// Emoji is the reaction name without colons, used by TriggerReaction.
	Emoji string `json:"emoji,omitempty"`
	// Authors are user IDs, bot IDs or app names whose posts the rule replies to. Empty matches everyone.
	Authors []string `json:"authors,omitempty"`
	// Prompt is given to the LLM. Mentions in it are written as "<@U0123>" or "<!subteam^S0123>".
	Prompt string `json:"prompt"`
	// PromptRichText is the prompt as the user wrote it in Slack, to show it and edit it again.
	PromptRichText json.RawMessage `json:"prompt_rich_text,omitempty"`
	// Mentions are the mentions in the prompt. A reply can mention only these.
	Mentions  []string  `json:"mentions,omitempty"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MatchesAuthor reports whether a post is a target of the rule.
// ids identify the author, such as the user ID, the bot ID and the app name. App names match case-insensitively.
func (r Rule) MatchesAuthor(ids ...string) bool {
	if len(r.Authors) == 0 {
		return true
	}
	for _, a := range r.Authors {
		for _, id := range ids {
			if id != "" && strings.EqualFold(a, id) {
				return true
			}
		}
	}
	return false
}

// MatchesReaction reports whether a reaction name, such as "+1::skin-tone-2", is the rule's emoji.
// same compares two emoji names, so the aliases of a standard emoji, such as "+1" and "thumbsup", can match.
func (r Rule) MatchesReaction(name string, same func(a, b string) bool) bool {
	base, _, _ := strings.Cut(name, "::")
	return r.Trigger == TriggerReaction && r.Emoji != "" && same(base, r.Emoji)
}

// Validate checks the fields a user fills in.
func (r Rule) Validate() error {
	if r.ChannelID == "" {
		return errors.New("channel is required")
	}
	if !r.Trigger.Valid() {
		return fmt.Errorf("unknown trigger %q", r.Trigger)
	}
	if r.Trigger == TriggerReaction && r.Emoji == "" {
		return errors.New("emoji is required for the reaction trigger")
	}
	if strings.TrimSpace(r.Prompt) == "" {
		return errors.New("prompt is required")
	}
	return nil
}

// NewID returns a random rule ID.
func NewID() string {
	return uuid.New().String()
}

// ErrNotFound is returned when a rule does not exist.
var ErrNotFound = errors.New("rule not found")

// Store keeps the rules.
type Store interface {
	ListByChannel(ctx context.Context, channelID string) ([]Rule, error)
	Get(ctx context.Context, id string) (Rule, error)
	Put(ctx context.Context, r Rule) error
	Delete(ctx context.Context, id string) error
	Close() error
}

// Open opens the store named by a URL: "file://<path>" for a JSON file or "sqlite://<path>" for SQLite.
// Both take an absolute path after the scheme, e.g. "file:///opt/data/rules.json".
func Open(ctx context.Context, url string) (Store, error) {
	if path, ok := strings.CutPrefix(url, "file://"); ok && path != "" {
		return OpenFile(path)
	}
	if path, ok := strings.CutPrefix(url, "sqlite://"); ok && path != "" {
		return OpenSQLite(ctx, path)
	}
	return nil, fmt.Errorf("unsupported store %q: use file://<path> or sqlite://<path>", url)
}

func sortRules(rules []Rule) {
	slices.SortFunc(rules, func(a, b Rule) int {
		if c := strings.Compare(string(a.Trigger), string(b.Trigger)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}
