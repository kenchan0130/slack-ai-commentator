package rule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"

	"github.com/google/renameio/v2"
)

// FileStore keeps the rules in one JSON file and in memory.
type FileStore struct {
	path  string
	mu    sync.RWMutex
	rules map[string]Rule
}

type fileContent struct {
	Rules []Rule `json:"rules"`
}

// OpenFile loads the rules from path. A missing file is an empty store.
// The file is written once here, so a directory that is missing or not writable fails at start, not when a rule is saved.
func OpenFile(path string) (*FileStore, error) {
	s := &FileStore{path: path, rules: map[string]Rule{}}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("os.ReadFile(): %w", err)
	default:
		var c fileContent
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("json.Unmarshal() %s: %w", path, err)
		}
		for _, r := range c.Rules {
			s.rules[r.ID] = r
		}
	}
	if err := s.save(); err != nil {
		return nil, fmt.Errorf("the rules can not be written to %s: %w", path, err)
	}
	return s, nil
}

func (s *FileStore) ListByChannel(_ context.Context, channelID string) ([]Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rules []Rule
	for _, r := range s.rules {
		if r.ChannelID == channelID {
			rules = append(rules, r)
		}
	}
	sortRules(rules)
	return rules, nil
}

func (s *FileStore) Get(_ context.Context, id string) (Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.rules[id]
	if !ok {
		return Rule{}, ErrNotFound
	}
	return r, nil
}

func (s *FileStore) Put(_ context.Context, r Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, existed := s.rules[r.ID]
	s.rules[r.ID] = r
	if err := s.save(); err != nil {
		if existed {
			s.rules[r.ID] = prev
		} else {
			delete(s.rules, r.ID)
		}
		return err
	}
	return nil
}

func (s *FileStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.rules[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.rules, id)
	if err := s.save(); err != nil {
		s.rules[id] = prev
		return err
	}
	return nil
}

func (s *FileStore) Close() error {
	return nil
}

// save replaces the file atomically, so a crash never leaves a half-written file.
func (s *FileStore) save() error {
	c := fileContent{Rules: make([]Rule, 0, len(s.rules))}
	for _, r := range s.rules {
		c.Rules = append(c.Rules, r)
	}
	sortRules(c.Rules)
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("json.MarshalIndent(): %w", err)
	}
	if err := renameio.WriteFile(s.path, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("renameio.WriteFile(): %w", err)
	}
	return nil
}
