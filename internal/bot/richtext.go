package bot

import (
	"fmt"
	"slices"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackutilsx"
)

// promptFromRichText converts the rich text of a prompt to the text given to the LLM, and returns the mentions in it.
// Users and user groups are written in the mention syntax of Slack, such as "<@U0123>" and "<!subteam^S0123>",
// so the LLM can write them in a reply, and only these mentions are allowed in the replies of the rule.
func promptFromRichText(rt slack.RichTextBlock) (string, []string) {
	var b strings.Builder
	var mentions []string
	mention := func(m string) {
		b.WriteString(m)
		if !slices.Contains(mentions, m) {
			mentions = append(mentions, m)
		}
	}
	writeElements := func(elements []slack.RichTextSectionElement) {
		for _, e := range elements {
			switch e := e.(type) {
			case *slack.RichTextSectionTextElement:
				if e.Style != nil && e.Style.Bold && e.Text != "" {
					b.WriteString("**" + e.Text + "**")
				} else {
					b.WriteString(e.Text)
				}
			case *slack.RichTextSectionUserElement:
				mention("<@" + e.UserID + ">")
			case *slack.RichTextSectionUserGroupElement:
				mention("<!subteam^" + e.UsergroupID + ">")
			case *slack.RichTextSectionChannelElement:
				b.WriteString("<#" + e.ChannelID + ">")
			case *slack.RichTextSectionEmojiElement:
				b.WriteString(":" + e.Name + ":")
			case *slack.RichTextSectionLinkElement:
				if e.Text != "" && e.Text != e.URL {
					fmt.Fprintf(&b, "%s (%s)", e.Text, e.URL)
				} else {
					b.WriteString(e.URL)
				}
			case *slack.RichTextSectionBroadcastElement:
				// @here and @channel do not notify in a thread, so they are kept as words.
				b.WriteString("@" + e.Range)
			}
		}
	}
	newline := func() {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
	}

	for _, el := range rt.Elements {
		switch el := el.(type) {
		case *slack.RichTextSection:
			writeElements(el.Elements)
		case *slack.RichTextList:
			newline()
			for i, item := range el.Elements {
				section, ok := item.(*slack.RichTextSection)
				if !ok {
					continue
				}
				b.WriteString(strings.Repeat("  ", el.Indent))
				if el.Style == slack.RTEListOrdered {
					fmt.Fprintf(&b, "%d. ", el.Offset+i+1)
				} else {
					b.WriteString("- ")
				}
				writeElements(section.Elements)
				b.WriteString("\n")
			}
		case *slack.RichTextQuote:
			newline()
			b.WriteString("> ")
			writeElements(el.Elements)
			b.WriteString("\n")
		case *slack.RichTextPreformatted:
			newline()
			b.WriteString("```\n")
			writeElements(el.Elements)
			b.WriteString("\n```\n")
		}
	}
	return strings.TrimSpace(b.String()), mentions
}

// richTextFromText is the rich text of a plain prompt, for a rule saved before prompts were rich text.
func richTextFromText(text string) *slack.RichTextBlock {
	return slack.NewRichTextBlock("", slack.NewRichTextSection(slack.NewRichTextSectionTextElement(text, nil)))
}

// escapeReply escapes a reply of the LLM, so it can not mention anyone, and then restores the allowed mentions,
// which are the ones in the prompt of the rule.
func escapeReply(text string, allowed []string) string {
	escaped := slackutilsx.EscapeMessage(text)
	for _, m := range allowed {
		escaped = strings.ReplaceAll(escaped, slackutilsx.EscapeMessage(m), m)
	}
	return strings.ReplaceAll(escaped, "&gt;", ">")
}

// mentionReference identifies a Slack mention in Markdown without triggering a second notification.
func mentionReference(mention string) string {
	if id, ok := strings.CutPrefix(mention, "<@"); ok && strings.HasSuffix(id, ">") {
		return "`@" + strings.TrimSuffix(id, ">") + "`"
	}
	if id, ok := strings.CutPrefix(mention, "<!subteam^"); ok && strings.HasSuffix(id, ">") {
		return "`@" + strings.TrimSuffix(id, ">") + "`"
	}
	return mention
}
