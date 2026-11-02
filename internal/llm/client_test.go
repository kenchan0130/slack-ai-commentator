package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type chatRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	Messages  []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func reply(content string) string {
	b, _ := json.Marshal(map[string]any{
		"id": "x", "object": "chat.completion", "created": 0, "model": "m",
		"choices": []map[string]any{{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}},
	})
	return string(b)
}

func TestComplete(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if a := r.Header.Get("Authorization"); a != "Bearer secret" {
			t.Errorf("Authorization = %q", a)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply("<think>\nhmm\n</think>\n\nanswer")))
	}))
	defer srv.Close()

	c := NewOpenAI(Options{BaseURL: srv.URL + "/v1/", Model: "m", APIKey: "secret", MaxTokens: 100})
	out, err := c.Complete(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatal(err)
	}
	if out != "answer" {
		t.Errorf("Complete() = %q, want the think block stripped", out)
	}
	if got.Model != "m" || got.MaxTokens != 100 || len(got.Messages) != 2 ||
		got.Messages[0].Role != "system" || got.Messages[0].Content != "sys" ||
		got.Messages[1].Role != "user" || got.Messages[1].Content != "usr" {
		t.Errorf("request = %+v", got)
	}
}

func TestCompleteRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 2 {
			w.Header().Set("Retry-After-Ms", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply("ok")))
	}))
	defer srv.Close()

	c := NewOpenAI(Options{BaseURL: srv.URL, APIKey: "k", Retries: 2})
	out, err := c.Complete(context.Background(), "s", "u")
	if err != nil || out != "ok" {
		t.Fatalf("Complete() = %q, %v", out, err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestCompleteErrors(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"bad request": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"bad model"}}`))
		},
		"only thinking": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(reply("<think>only thinking</think>")))
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(handler)
			defer srv.Close()
			c := NewOpenAI(Options{BaseURL: srv.URL, APIKey: "k"})
			if _, err := c.Complete(context.Background(), "s", "u"); err == nil || (name == "bad request" && !strings.Contains(err.Error(), "400")) {
				t.Fatalf("Complete() error = %v", err)
			}
		})
	}
}
