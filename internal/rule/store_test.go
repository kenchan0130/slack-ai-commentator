package rule

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openStores(t *testing.T) map[string]func() Store {
	t.Helper()
	dir := t.TempDir()
	return map[string]func() Store{
		"file": func() Store {
			s, err := Open(context.Background(), "file://"+filepath.Join(dir, "rules.json"))
			if err != nil {
				t.Fatal(err)
			}
			return s
		},
		"sqlite": func() Store {
			s, err := Open(context.Background(), "sqlite://"+filepath.Join(dir, "rules.db"))
			if err != nil {
				t.Fatal(err)
			}
			return s
		},
	}
}

func TestStore(t *testing.T) {
	for name, open := range openStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s := open()

			if _, err := s.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("Get(missing) = %v, want ErrNotFound", err)
			}
			if err := s.Delete(ctx, "missing"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("Delete(missing) = %v, want ErrNotFound", err)
			}

			r1 := Rule{ID: "a", ChannelID: "C1", Trigger: TriggerReaction, Emoji: "mag", Authors: []string{"B1"}, Prompt: "p1", UpdatedAt: time.Unix(1, 0).UTC()}
			r2 := Rule{ID: "b", ChannelID: "C1", Trigger: TriggerMessage, Prompt: "p2"}
			r3 := Rule{ID: "c", ChannelID: "C2", Trigger: TriggerMention, Prompt: "p3"}
			for _, r := range []Rule{r1, r2, r3} {
				if err := s.Put(ctx, r); err != nil {
					t.Fatal(err)
				}
			}

			r1.Prompt = "updated"
			if err := s.Put(ctx, r1); err != nil {
				t.Fatal(err)
			}
			if err := s.Delete(ctx, "c"); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}

			// Reopen to check the rules were persisted.
			s = open()
			defer s.Close()
			got, err := s.ListByChannel(ctx, "C1")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
				t.Fatalf("ListByChannel(C1) = %+v", got)
			}
			if got[1].Prompt != "updated" || got[1].Emoji != "mag" || len(got[1].Authors) != 1 || !got[1].UpdatedAt.Equal(r1.UpdatedAt) {
				t.Fatalf("rule a = %+v", got[1])
			}
			if got, _ := s.ListByChannel(ctx, "C2"); len(got) != 0 {
				t.Fatalf("ListByChannel(C2) = %+v, want empty", got)
			}
		})
	}
}

func TestOpenFileFailsAtStart(t *testing.T) {
	// A missing directory fails when the store is opened, not when the first rule is saved.
	if _, err := OpenFile(filepath.Join(t.TempDir(), "missing", "rules.json")); err == nil {
		t.Fatal("OpenFile() succeeded for a missing directory")
	}
	// A new file is created, so the store is known to be writable.
	path := filepath.Join(t.TempDir(), "rules.json")
	if _, err := OpenFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the file was not created: %v", err)
	}
}

func TestOpenUnsupported(t *testing.T) {
	for _, url := range []string{"", "rules.json", "file://", "postgres://x"} {
		if _, err := Open(context.Background(), url); err == nil {
			t.Errorf("Open(%q) succeeded", url)
		}
	}
}

func TestMatches(t *testing.T) {
	r := Rule{Trigger: TriggerReaction, Emoji: "+1", Authors: []string{"U1", "B1", "RSS"}}
	if !r.MatchesAuthor("U1", "") || !r.MatchesAuthor("", "B1") || !r.MatchesAuthor("", "B9", "rss") || r.MatchesAuthor("U2", "B2", "Other") || r.MatchesAuthor("", "") {
		t.Error("MatchesAuthor with authors")
	}
	if !(Rule{}).MatchesAuthor("", "") {
		t.Error("a rule without authors should match everyone")
	}
	if !r.MatchesReaction("+1") || !r.MatchesReaction("+1::skin-tone-2") || r.MatchesReaction("-1") {
		t.Error("MatchesReaction")
	}
	if (Rule{Trigger: TriggerMessage, Emoji: "+1"}).MatchesReaction("+1") {
		t.Error("only reaction rules match reactions")
	}
}

func TestValidate(t *testing.T) {
	ok := Rule{ChannelID: "C1", Trigger: TriggerMention, Prompt: "p"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, r := range []Rule{
		{Trigger: TriggerMention, Prompt: "p"},
		{ChannelID: "C1", Trigger: "x", Prompt: "p"},
		{ChannelID: "C1", Trigger: TriggerReaction, Prompt: "p"},
		{ChannelID: "C1", Trigger: TriggerMention, Prompt: "  "},
	} {
		if r.Validate() == nil {
			t.Errorf("Validate(%+v) succeeded", r)
		}
	}
}
