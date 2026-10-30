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

// customEmojiNames returns the custom emoji of the workspace. They are cached, because the list can be long.
func (b *Bot) customEmojiNames(ctx context.Context) []string {
	if item := b.emojiCache.Get("custom"); item != nil {
		return item.Value()
	}
	m, err := b.api.GetEmojiContext(ctx)
	if err != nil {
		b.logger.Printf("slack.Client#GetEmojiContext(): %v", err)
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
