package bot

import (
	"context"
	"fmt"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

// ackTimeout keeps the work done before an acknowledgement within the 3 seconds Slack waits for it.
const ackTimeout = 2500 * time.Millisecond

// Listen handles Socket Mode events until ctx is done.
// Every request is handled in its own goroutine, so a slow one, such as listing the custom emoji, does not delay the others.
func (b *Bot) Listen(ctx context.Context, client *socketmode.Client) {
	for {
		var evt socketmode.Event
		select {
		case <-ctx.Done():
			return
		case evt = <-client.Events:
		}

		switch evt.Type {
		case socketmode.EventTypeConnecting:
			b.logger.Info("connecting to Slack")
		case socketmode.EventTypeConnected:
			b.logger.Info("connected to Slack")
		case socketmode.EventTypeConnectionError, socketmode.EventTypeInvalidAuth, socketmode.EventTypeIncomingError, socketmode.EventTypeErrorBadMessage:
			b.logger.Warn("socket mode error", "type", evt.Type, "error", fmt.Sprint(evt.Data))

		case socketmode.EventTypeEventsAPI:
			b.ack(client, evt.Request, nil)
			go func(req *socketmode.Request) {
				if err := b.HandleEvent(ctx, req.Payload); err != nil {
					b.logger.Error("failed to handle an event", "error", err)
				}
			}(evt.Request)

		case socketmode.EventTypeSlashCommand:
			cmd, ok := evt.Data.(slack.SlashCommand)
			if !ok {
				b.ack(client, evt.Request, nil)
				continue
			}
			// The acknowledgement carries the response, so it is sent after the command is handled.
			go b.handleThenAck(ctx, client, evt.Request, "slash command", func(ctx context.Context) (any, error) {
				return b.HandleSlashCommand(ctx, cmd)
			})

		case socketmode.EventTypeInteractive:
			cb, ok := evt.Data.(slack.InteractionCallback)
			if !ok {
				b.ack(client, evt.Request, nil)
				continue
			}
			if cb.Type == slack.InteractionTypeViewSubmission || cb.Type == slack.InteractionTypeBlockSuggestion {
				// The acknowledgement closes a modal or shows its errors, and carries the options of an external select.
				go b.handleThenAck(ctx, client, evt.Request, string(cb.Type), func(ctx context.Context) (any, error) {
					return b.HandleInteraction(ctx, cb)
				})
				continue
			}
			b.ack(client, evt.Request, nil)
			go func() {
				if _, err := b.HandleInteraction(ctx, cb); err != nil {
					b.logger.Error("failed to handle an interaction", "type", cb.Type, "error", err)
				}
			}()
		}
	}
}

// handleThenAck runs handle within ackTimeout and acknowledges the request with its payload.
func (b *Bot) handleThenAck(ctx context.Context, client *socketmode.Client, req *socketmode.Request, kind string, handle func(context.Context) (any, error)) {
	ctx, cancel := context.WithTimeout(ctx, ackTimeout)
	defer cancel()
	payload, err := handle(ctx)
	if err != nil {
		b.logger.Error("failed to handle a request", "type", kind, "error", err)
	}
	b.ack(client, req, payload)
}

func (b *Bot) ack(client *socketmode.Client, req *socketmode.Request, payload any) {
	if req == nil {
		return
	}
	var err error
	if payload == nil {
		err = client.Ack(*req)
	} else {
		err = client.Ack(*req, payload)
	}
	if err != nil {
		b.logger.Error("failed to acknowledge a request", "type", req.Type, "error", err)
	}
}
