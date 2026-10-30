package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/kenchan0130/slack-ai-commentator/internal/bot"
	"github.com/kenchan0130/slack-ai-commentator/internal/config"
	"github.com/kenchan0130/slack-ai-commentator/internal/llm"
	"github.com/kenchan0130/slack-ai-commentator/internal/locale"
	"github.com/kenchan0130/slack-ai-commentator/internal/rule"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadFromEnv(ctx)
	if err != nil {
		return fmt.Errorf("load the configuration: %w", err)
	}

	store, err := rule.Open(ctx, cfg.Store)
	if err != nil {
		return fmt.Errorf("open the store: %w", err)
	}
	defer store.Close()

	localizer, err := locale.New(cfg.UILanguage)
	if err != nil {
		return fmt.Errorf("UI_LANGUAGE: %w", err)
	}

	logger := log.New(os.Stderr, "", log.LstdFlags)
	api := slack.New(cfg.SlackBotToken, slack.OptionAppLevelToken(cfg.SlackAppToken), slack.OptionDebug(cfg.Debug))
	auth, err := api.AuthTestContext(ctx)
	if err != nil {
		return fmt.Errorf("slack.Client#AuthTestContext(): %w", err)
	}
	logger.Printf("running as %s (user %s, bot %s)", auth.User, auth.UserID, auth.BotID)

	llmOpts := llm.Options{
		BaseURL:   cfg.LLMURL,
		Model:     cfg.LLMModel,
		APIKey:    cfg.LLMAPIKey,
		Region:    cfg.BedrockRegion,
		MaxTokens: cfg.LLMMaxTokens,
		Timeout:   cfg.LLMTimeout,
		Retries:   cfg.LLMRetries,
	}
	var completer bot.Completer = llm.NewOpenAI(llmOpts)
	if cfg.LLMProvider == config.ProviderBedrock {
		if completer, err = llm.NewBedrock(ctx, llmOpts); err != nil {
			return fmt.Errorf("llm.NewBedrock(): %w", err)
		}
	}
	logger.Printf("using %s (model %q)", cfg.LLMProvider, cfg.LLMModel)

	b := bot.New(api,
		completer,
		store,
		bot.Identity{UserID: auth.UserID, BotID: auth.BotID},
		bot.Options{QueueSize: cfg.QueueSize, MaxInputChars: cfg.MaxInputChars, Localizer: localizer},
		logger,
	)

	var wg sync.WaitGroup
	for range cfg.LLMConcurrency {
		wg.Go(func() { b.Work(ctx) })
	}

	client := socketmode.New(api, socketmode.OptionDebug(cfg.Debug))
	wg.Go(func() { b.Listen(ctx, client) })
	err = client.RunContext(ctx)
	// A signal stopped the app when ctx is done before stop below, which cancels it in any case.
	stopped := ctx.Err() != nil
	stop()
	wg.Wait()
	if stopped {
		return nil
	}
	return fmt.Errorf("socketmode.Client#RunContext(): %w", err)
}
