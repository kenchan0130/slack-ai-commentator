package bot

import (
	"html"
	"slices"
	"strings"

	"github.com/slack-go/slack"
)

// messageText returns the readable text of a post. Apps such as the RSS app put their body in attachments,
// and some apps put a short summary in the text and the details in blocks, so all of them are read.
// A text that appears twice is kept once.
func messageText(m slack.Msg) string {
	var parts []string
	add := func(s string) {
		s = strings.TrimSpace(html.UnescapeString(s))
		if s != "" && !slices.Contains(parts, s) {
			parts = append(parts, s)
		}
	}

	add(m.Text)
	for _, a := range m.Attachments {
		add(a.Pretext)
		add(a.AuthorName)
		add(a.Title)
		add(a.TitleLink)
		add(a.Text)
		for _, f := range a.Fields {
			add(strings.TrimSpace(f.Title + " " + f.Value))
		}
		if a.Pretext == "" && a.Title == "" && a.Text == "" {
			add(a.Fallback)
		}
	}
	for _, b := range m.Blocks.BlockSet {
		for _, t := range blockTexts(b) {
			add(t)
		}
	}
	return strings.Join(parts, "\n")
}

func blockTexts(b slack.Block) []string {
	var texts []string
	addObj := func(o *slack.TextBlockObject) {
		if o != nil {
			texts = append(texts, o.Text)
		}
	}
	switch b := b.(type) {
	case *slack.SectionBlock:
		addObj(b.Text)
		for _, f := range b.Fields {
			addObj(f)
		}
	case *slack.HeaderBlock:
		addObj(b.Text)
	case *slack.ContextBlock:
		for _, e := range b.ContextElements.Elements {
			if o, ok := e.(*slack.TextBlockObject); ok {
				addObj(o)
			}
		}
	case *slack.MarkdownBlock:
		texts = append(texts, b.Text)
	}
	return texts
}
