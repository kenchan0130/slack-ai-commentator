package bot

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/jellydator/ttlcache/v3"
	"github.com/kyokomi/emoji/v2"
	"github.com/slack-go/slack"
)

// maxEmojiOptions is the most options Slack shows for an external select.
const maxEmojiOptions = 100

// standardEmojiNames are the names of the standard emoji without colons, such as "mag".
var standardEmojiNames = sync.OnceValue(func() []string {
	names := make([]string, 0, len(emoji.CodeMap()))
	for code := range emoji.CodeMap() {
		names = append(names, strings.Trim(code, ":"))
	}
	slices.Sort(names)
	return names
})

// emojiCharacter returns the characters of a standard emoji name, or "" for a custom emoji.
// The variation selector is removed, because the aliases of an emoji may differ only in it.
func emojiCharacter(name string) string {
	return strings.ReplaceAll(emoji.CodeMap()[":"+name+":"], "️", "")
}

// sameEmoji reports whether two names are the same emoji. A standard emoji has aliases, such as "+1", "thumbsup"
// and "thumbs_up", and the name the rule stores may not be the one Slack sends with a reaction, so standard emoji
// are compared by their characters. Custom emoji are compared by their names.
func sameEmoji(a, b string) bool {
	if a == b {
		return true
	}
	ca := emojiCharacter(a)
	return ca != "" && ca == emojiCharacter(b)
}

// customEmojiNames returns the custom emoji of the workspace. They are cached, because the list can be long.
func (b *Bot) customEmojiNames(ctx context.Context) []string {
	if item := b.emojiCache.Get("custom"); item != nil {
		return item.Value()
	}
	m, err := b.api.GetEmojiContext(ctx)
	if err != nil {
		b.logger.Error("failed to list the custom emoji", "error", err)
		return nil
	}
	names := slices.Sorted(maps.Keys(m))
	b.emojiCache.Set("custom", names, ttlcache.DefaultTTL)
	return names
}

// emojiOptions returns the emoji matching query: names starting with it first, and then names containing it.
func (b *Bot) emojiOptions(ctx context.Context, query string) []*slack.OptionBlockObject {
	q := strings.ToLower(strings.Trim(strings.TrimSpace(query), ":"))
	var prefix, contains []string
	seen := map[string]bool{}
	for _, name := range slices.Concat(b.customEmojiNames(ctx), standardEmojiNames()) {
		if seen[name] {
			continue
		}
		seen[name] = true
		switch {
		case strings.HasPrefix(name, q):
			prefix = append(prefix, name)
		case strings.Contains(name, q):
			contains = append(contains, name)
		}
	}
	matched := slices.Concat(prefix, contains)
	if len(matched) > maxEmojiOptions {
		matched = matched[:maxEmojiOptions]
	}
	options := make([]*slack.OptionBlockObject, 0, len(matched))
	for _, name := range matched {
		options = append(options, emojiOption(name))
	}
	return options
}

// emojiOption shows the emoji itself next to its name.
func emojiOption(name string) *slack.OptionBlockObject {
	return slack.NewOptionBlockObject(name, slack.NewTextBlockObject(slack.PlainTextType, ":"+name+": "+name, true, false), nil)
}
