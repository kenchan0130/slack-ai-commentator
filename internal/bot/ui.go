package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kenchan0130/slack-ai-commentator/internal/rule"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackutilsx"
)

const (
	actionAddRule    = "add_rule"
	actionListRules  = "list_rules"
	actionEditRule   = "edit_rule"
	actionDeleteRule = "delete_rule"

	callbackRuleModal = "rule_modal"

	blockTrigger = "trigger"
	blockEmoji   = "emoji"
	blockAuthors = "authors"
	blockPrompt  = "prompt"
	actionInput  = "input"
)

// triggerMessages are the message IDs of the trigger labels.
var triggerMessages = map[rule.Trigger]string{
	rule.TriggerMention:  "TriggerMention",
	rule.TriggerMessage:  "TriggerMessage",
	rule.TriggerReaction: "TriggerReaction",
}

func plain(s string) *slack.TextBlockObject {
	return slack.NewTextBlockObject(slack.PlainTextType, s, false, false)
}

func mrkdwn(s string) *slack.TextBlockObject {
	return slack.NewTextBlockObject(slack.MarkdownType, s, false, false)
}

func (b *Bot) setupBlocks(channelID string) []slack.Block {
	return []slack.Block{
		slack.NewSectionBlock(mrkdwn(b.t.T("Setup")), nil, nil),
		slack.NewActionBlock("setup", slack.NewButtonBlockElement(actionAddRule, channelID, plain(b.t.T("AddRule"))).WithStyle(slack.StylePrimary)),
	}
}

// shorten cuts s to n characters, so a long input can not exceed the text limits of Slack.
func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// maxShownRichPromptChars is the longest prompt shown as the rich text it was written in. A longer one is cut.
const maxShownRichPromptChars = 1000

// promptRichText returns the rich text of a prompt, or nil when the rule has none.
func promptRichText(r rule.Rule) *slack.RichTextBlock {
	if len(r.PromptRichText) == 0 {
		return nil
	}
	var rt slack.RichTextBlock
	if err := json.Unmarshal(r.PromptRichText, &rt); err != nil || len(rt.Elements) == 0 {
		return nil
	}
	return &rt
}

// describe shows a rule: its trigger and authors, and its prompt.
// The prompt is shown as the rich text it was written in, with its mentions; describe is only used in messages that
// only one user sees, which notify nobody. A long prompt, or one without rich text, is cut and shown as preformatted
// text, which Slack shows as it is, so it is never read as mrkdwn nor mentions anyone.
func (b *Bot) describe(r rule.Rule) []slack.Block {
	trigger := b.t.T(triggerMessages[r.Trigger])
	if r.Trigger == rule.TriggerReaction {
		trigger += fmt.Sprintf(" :%s:", r.Emoji)
	}
	authors := b.t.T("Everyone")
	if len(r.Authors) > 0 {
		authors = shorten(strings.Join(r.Authors, ", "), 200)
	}
	// Plain text keeps user-supplied author names from creating mentions.
	summary := b.t.T("RuleSummary", map[string]any{"Trigger": trigger, "Authors": authors})
	summaryBlock := slack.NewSectionBlock(slack.NewTextBlockObject(slack.PlainTextType, summary, true, false), nil, nil)
	if rt := promptRichText(r); rt != nil && len([]rune(r.Prompt)) <= maxShownRichPromptChars {
		return []slack.Block{summaryBlock, rt}
	}
	prompt := &slack.RichTextPreformatted{
		Type:     slack.RTEPreformatted,
		Elements: []slack.RichTextSectionElement{slack.NewRichTextSectionTextElement(shorten(r.Prompt, 200), nil)},
	}
	return []slack.Block{
		summaryBlock,
		slack.NewRichTextBlock("", prompt),
	}
}

// maxRules is the most rules a channel can have. It keeps the list within the 50 blocks a message may have:
// 4 blocks for each rule and 2 more. Every rule is then listed, so it can be edited and deleted.
const maxRules = 12

// channelIsFull reports whether r is a new rule for a channel that has maxRules already.
func (b *Bot) channelIsFull(ctx context.Context, r rule.Rule) (bool, error) {
	rules, err := b.store.ListByChannel(ctx, r.ChannelID)
	if err != nil {
		return false, fmt.Errorf("rule.Store#ListByChannel(): %w", err)
	}
	for _, existing := range rules {
		if existing.ID == r.ID {
			return false, nil
		}
	}
	return len(rules) >= maxRules, nil
}

// listBlocks shows the rules of a channel with buttons to edit them.
func (b *Bot) listBlocks(channelID string, rules []rule.Rule) []slack.Block {
	header := b.t.T("ListHeader", map[string]any{"Channel": channelID, "Count": len(rules)})
	// A channel has maxRules at most, but the list is cut in case the store was edited by hand.
	if len(rules) > maxRules {
		header += b.t.T("ListTruncated", map[string]any{"Count": maxRules})
		rules = rules[:maxRules]
	}
	blocks := make([]slack.Block, 0, 1+4*len(rules)+1)
	blocks = append(blocks, slack.NewSectionBlock(mrkdwn(header), nil, nil))
	for _, r := range rules {
		blocks = append(blocks, slack.NewDividerBlock())
		blocks = append(blocks, b.describe(r)...)
		blocks = append(blocks,
			slack.NewActionBlock("rule_"+r.ID,
				slack.NewButtonBlockElement(actionEditRule, r.ID, plain(b.t.T("Edit"))),
				slack.NewButtonBlockElement(actionDeleteRule, r.ID, plain(b.t.T("Delete"))).WithStyle(slack.StyleDanger).
					WithConfirm(slack.NewConfirmationBlockObject(plain(b.t.T("DeleteConfirmTitle")), plain(b.t.T("DeleteConfirmText")), plain(b.t.T("Delete")), plain(b.t.T("Cancel")))),
			),
		)
	}
	blocks = append(blocks, slack.NewActionBlock("add", slack.NewButtonBlockElement(actionAddRule, channelID, plain(b.t.T("AddRule"))).WithStyle(slack.StylePrimary)))
	return blocks
}

type modalMetadata struct {
	ChannelID string `json:"channel_id"`
	RuleID    string `json:"rule_id,omitempty"`
	// Emoji keeps the chosen emoji while the emoji field is hidden for another trigger.
	Emoji string `json:"emoji,omitempty"`
}

// ruleModal shows a rule to edit. A rule without an ID is a new rule.
// The emoji field is only shown for the reaction trigger, and the modal is updated when the trigger is changed.
func (b *Bot) ruleModal(channelID string, r *rule.Rule) slack.ModalViewRequest {
	if r == nil {
		r = &rule.Rule{Trigger: rule.TriggerReaction}
	}
	meta := modalMetadata{ChannelID: channelID, RuleID: r.ID, Emoji: r.Emoji}
	metaJSON, _ := json.Marshal(meta)

	options := make([]*slack.OptionBlockObject, 0, len(rule.Triggers))
	var initial *slack.OptionBlockObject
	for _, t := range rule.Triggers {
		o := slack.NewOptionBlockObject(string(t), plain(b.t.T(triggerMessages[t])), nil)
		options = append(options, o)
		if t == r.Trigger {
			initial = o
		}
	}
	trigger := slack.NewOptionsSelectBlockElement(slack.OptTypeStatic, plain(b.t.T("ModalTriggerPlaceholder")), actionInput, options...)
	trigger.InitialOption = initial

	authors := slack.NewPlainTextInputBlockElement(plain("U0123ABCD, B0123ABCD, RSS"), actionInput)
	authors.InitialValue = strings.Join(r.Authors, ", ")
	// Rich text lets the user mention users and user groups with @, which a reply of the rule can then mention.
	prompt := slack.NewRichTextInputBlockElement(plain(b.t.T("ModalPromptPlaceholder")), actionInput)
	if rt := promptRichText(*r); rt != nil {
		prompt.InitialValue = rt
	} else if r.Prompt != "" {
		prompt.InitialValue = richTextFromText(r.Prompt)
	}

	authorsBlock := slack.NewInputBlock(blockAuthors, plain(b.t.T("ModalAuthors")), plain(b.t.T("ModalAuthorsHint")), authors)
	authorsBlock.Optional = true

	blocks := []slack.Block{
		slack.NewSectionBlock(mrkdwn(b.t.T("ModalIntro", map[string]any{"Channel": channelID})), nil, nil),
		slack.NewInputBlock(blockTrigger, plain(b.t.T("ModalTrigger")), nil, trigger).WithDispatchAction(true),
	}
	if r.Trigger == rule.TriggerReaction {
		// The options are searched as the user types, from the custom and standard emoji.
		emoji := slack.NewOptionsSelectBlockElement(slack.OptTypeExternal, plain(b.t.T("ModalEmojiPlaceholder")), actionInput)
		emoji.MinQueryLength = new(0)
		if r.Emoji != "" {
			emoji.InitialOption = emojiOption(r.Emoji)
		}
		blocks = append(blocks, slack.NewInputBlock(blockEmoji, plain(b.t.T("ModalEmoji")), plain(b.t.T("ModalEmojiHint")), emoji))
	}
	blocks = append(blocks,
		authorsBlock,
		slack.NewInputBlock(blockPrompt, plain(b.t.T("ModalPrompt")), plain(b.t.T("ModalPromptHint")), prompt),
	)

	title := b.t.T("AddRule")
	if meta.RuleID != "" {
		title = b.t.T("EditRule")
	}
	return slack.ModalViewRequest{
		Type:            slack.VTModal,
		CallbackID:      callbackRuleModal,
		PrivateMetadata: string(metaJSON),
		Title:           plain(title),
		Submit:          plain(b.t.T("Save")),
		Close:           plain(b.t.T("Cancel")),
		Blocks:          slack.Blocks{BlockSet: blocks},
	}
}

// ruleFromView reads the rule being edited in a modal. It keeps the emoji for any trigger, so it is not lost
// when the trigger is changed back and forth.
func ruleFromView(view slack.View) (rule.Rule, error) {
	var meta modalMetadata
	if err := json.Unmarshal([]byte(view.PrivateMetadata), &meta); err != nil {
		return rule.Rule{}, fmt.Errorf("json.Unmarshal() private metadata: %w", err)
	}
	value := func(block string) slack.BlockAction {
		if view.State == nil {
			return slack.BlockAction{}
		}
		return view.State.Values[block][actionInput]
	}

	r := rule.Rule{
		ID:        meta.RuleID,
		ChannelID: meta.ChannelID,
		Trigger:   rule.Trigger(value(blockTrigger).SelectedOption.Value),
		Emoji:     value(blockEmoji).SelectedOption.Value,
	}
	if r.Emoji == "" {
		r.Emoji = meta.Emoji
	}
	if rt := value(blockPrompt).RichTextValue; len(rt.Elements) > 0 {
		r.Prompt, r.Mentions = promptFromRichText(rt)
		if r.Prompt != "" {
			b, err := json.Marshal(rt)
			if err != nil {
				return rule.Rule{}, fmt.Errorf("json.Marshal() prompt: %w", err)
			}
			r.PromptRichText = b
		}
	}
	// Only commas and newlines separate the authors, because an app name may have spaces, such as "Google Calendar".
	for _, a := range strings.FieldsFunc(value(blockAuthors).Value, func(c rune) bool { return c == ',' || c == '\n' }) {
		if a = strings.TrimSpace(a); a != "" && !slices.Contains(r.Authors, a) {
			r.Authors = append(r.Authors, a)
		}
	}
	return r, nil
}

// parseRuleModal reads a submitted modal. It returns errors keyed by block ID for Slack to show.
func (b *Bot) parseRuleModal(view slack.View, userID string, now time.Time) (rule.Rule, map[string]string, error) {
	r, err := ruleFromView(view)
	if err != nil {
		return rule.Rule{}, nil, err
	}
	if r.ID == "" {
		r.ID = rule.NewID()
	}
	r.UpdatedBy = userID
	r.UpdatedAt = now
	if r.Trigger != rule.TriggerReaction {
		r.Emoji = ""
	}

	errs := map[string]string{}
	if r.Trigger == rule.TriggerReaction && r.Emoji == "" {
		errs[blockEmoji] = b.t.T("ErrorEmojiRequired")
	}
	if r.Prompt == "" {
		errs[blockPrompt] = b.t.T("ErrorPromptRequired")
	}
	if len(errs) > 0 {
		return r, errs, nil
	}
	if err := r.Validate(); err != nil {
		return r, map[string]string{blockTrigger: err.Error()}, nil
	}
	return r, nil, nil
}

func ephemeral(blocks ...slack.Block) map[string]any {
	return map[string]any{"response_type": "ephemeral", "blocks": blocks}
}

// helpBlocks shows the subcommands, with buttons for the common ones.
func (b *Bot) helpBlocks(command, channelID string) []slack.Block {
	text := b.t.T("Help", map[string]any{"Command": slackutilsx.EscapeMessage(command)})
	return []slack.Block{
		slack.NewSectionBlock(mrkdwn(text), nil, nil),
		slack.NewActionBlock("help",
			slack.NewButtonBlockElement(actionListRules, channelID, plain(b.t.T("ListRules"))),
			slack.NewButtonBlockElement(actionAddRule, channelID, plain(b.t.T("AddRule"))).WithStyle(slack.StylePrimary),
		),
	}
}

// HandleSlashCommand returns the ephemeral response to the slash command, or opens the modal for "add".
// The command alone, "help" and unknown subcommands show the usage.
func (b *Bot) HandleSlashCommand(ctx context.Context, cmd slack.SlashCommand) (any, error) {
	sub, _, _ := strings.Cut(strings.TrimSpace(cmd.Text), " ")
	switch strings.ToLower(sub) {
	case "list":
		rules, err := b.store.ListByChannel(ctx, cmd.ChannelID)
		if err != nil {
			return ephemeral(slack.NewSectionBlock(mrkdwn(b.t.T("ErrorReadRules")), nil, nil)), fmt.Errorf("rule.Store#ListByChannel(): %w", err)
		}
		return ephemeral(b.listBlocks(cmd.ChannelID, rules)...), nil
	case "add":
		if _, err := b.api.OpenViewContext(ctx, cmd.TriggerID, b.ruleModal(cmd.ChannelID, nil)); err != nil {
			return ephemeral(slack.NewSectionBlock(mrkdwn(b.t.T("ErrorOpenModal")), nil, nil)), fmt.Errorf("slack.Client#OpenViewContext(): %w", err)
		}
		return nil, nil
	case "", "help":
		return ephemeral(b.helpBlocks(cmd.Command, cmd.ChannelID)...), nil
	default:
		unknown := b.t.T("UnknownCommand", map[string]any{"Text": slackutilsx.EscapeMessage(sub)})
		return ephemeral(append([]slack.Block{slack.NewSectionBlock(mrkdwn(unknown), nil, nil)}, b.helpBlocks(cmd.Command, cmd.ChannelID)...)...), nil
	}
}

// HandleInteraction handles button clicks and modal submissions.
// It returns the payload to acknowledge the request with, which is only set for a modal with errors.
func (b *Bot) HandleInteraction(ctx context.Context, cb slack.InteractionCallback) (any, error) {
	switch cb.Type {
	case slack.InteractionTypeBlockActions:
		return nil, b.handleBlockActions(ctx, cb)
	case slack.InteractionTypeBlockSuggestion:
		if cb.BlockID != blockEmoji {
			return slack.OptionsResponse{}, nil
		}
		return slack.OptionsResponse{Options: b.emojiOptions(ctx, cb.Value)}, nil
	case slack.InteractionTypeViewSubmission:
		if cb.View.CallbackID != callbackRuleModal {
			return nil, nil
		}
		r, errs, err := b.parseRuleModal(cb.View, cb.User.ID, time.Now())
		if err != nil {
			return nil, err
		}
		if len(errs) > 0 {
			return slack.NewErrorsViewSubmissionResponse(errs), nil
		}
		if full, err := b.channelIsFull(ctx, r); err != nil || full {
			msg := b.t.T("ErrorTooManyRules", map[string]any{"Count": maxRules})
			if err != nil {
				msg = b.t.T("ErrorSaveRule")
			}
			return slack.NewErrorsViewSubmissionResponse(map[string]string{blockTrigger: msg}), err
		}
		if err := b.store.Put(ctx, r); err != nil {
			return slack.NewErrorsViewSubmissionResponse(map[string]string{blockPrompt: b.t.T("ErrorSaveRule")}),
				fmt.Errorf("rule.Store#Put(): %w", err)
		}
		b.logger.Info("saved a rule", "rule", r.ID, "trigger", r.Trigger, "channel", r.ChannelID, "user", cb.User.ID)
		// The modal waits for the acknowledgement, so the confirmation is posted after it,
		// without the deadline of the acknowledgement.
		ctx := context.WithoutCancel(ctx)
		go func() {
			blocks := append([]slack.Block{slack.NewSectionBlock(mrkdwn(b.t.T("Saved")), nil, nil)}, b.describe(r)...)
			if _, err := b.api.PostEphemeralContext(ctx, r.ChannelID, cb.User.ID, slack.MsgOptionText(b.t.T("Saved"), false), slack.MsgOptionBlocks(blocks...)); err != nil {
				b.logger.Error("failed to confirm the saved rule", "rule", r.ID, "error", err)
			}
		}()
	}
	return nil, nil
}

func (b *Bot) handleBlockActions(ctx context.Context, cb slack.InteractionCallback) error {
	var errs []error
	for _, a := range cb.ActionCallback.BlockActions {
		// Changing the trigger in the modal shows or hides the emoji field.
		if cb.View.ID != "" && a.BlockID == blockTrigger {
			r, err := ruleFromView(cb.View)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if _, err := b.api.UpdateViewContext(ctx, b.ruleModal(r.ChannelID, &r), "", cb.View.Hash, cb.View.ID); err != nil {
				errs = append(errs, fmt.Errorf("slack.Client#UpdateViewContext(): %w", err))
			}
			continue
		}
		switch a.ActionID {
		case actionAddRule:
			if _, err := b.api.OpenViewContext(ctx, cb.TriggerID, b.ruleModal(a.Value, nil)); err != nil {
				errs = append(errs, fmt.Errorf("slack.Client#OpenViewContext(): %w", err))
			}
		case actionListRules:
			if cb.ResponseURL == "" {
				continue
			}
			if err := b.replaceWithList(ctx, cb.ResponseURL, a.Value); err != nil {
				errs = append(errs, err)
			}
		case actionEditRule:
			r, err := b.store.Get(ctx, a.Value)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule.Store#Get(): %w", err))
				continue
			}
			if _, err := b.api.OpenViewContext(ctx, cb.TriggerID, b.ruleModal(r.ChannelID, &r)); err != nil {
				errs = append(errs, fmt.Errorf("slack.Client#OpenViewContext(): %w", err))
			}
		case actionDeleteRule:
			r, err := b.store.Get(ctx, a.Value)
			if errors.Is(err, rule.ErrNotFound) {
				continue
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("rule.Store#Get(): %w", err))
				continue
			}
			if err := b.store.Delete(ctx, r.ID); err != nil && !errors.Is(err, rule.ErrNotFound) {
				errs = append(errs, fmt.Errorf("rule.Store#Delete(): %w", err))
				continue
			}
			b.logger.Info("deleted a rule", "rule", r.ID, "trigger", r.Trigger, "channel", r.ChannelID, "user", cb.User.ID)
			if cb.ResponseURL == "" {
				continue
			}
			if err := b.replaceWithList(ctx, cb.ResponseURL, r.ChannelID); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// replaceWithList replaces the ephemeral message of a button with the rules of the channel.
func (b *Bot) replaceWithList(ctx context.Context, responseURL, channelID string) error {
	rules, err := b.store.ListByChannel(ctx, channelID)
	if err != nil {
		return fmt.Errorf("rule.Store#ListByChannel(): %w", err)
	}
	if err := b.postResponse(ctx, responseURL, &slack.WebhookMessage{
		ResponseType:    slack.ResponseTypeEphemeral,
		ReplaceOriginal: true,
		Blocks:          &slack.Blocks{BlockSet: b.listBlocks(channelID, rules)},
	}); err != nil {
		return fmt.Errorf("slack.PostWebhookContext(): %w", err)
	}
	return nil
}
