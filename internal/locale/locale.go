package locale

import (
	"embed"
	"fmt"
	"io/fs"
	"slices"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed messages/*.toml
var messages embed.FS

// Languages are the languages the app speaks.
var Languages = []string{"en", "ja"}

// Localizer returns the messages of one language.
type Localizer struct {
	l *i18n.Localizer
}

// New returns a localizer for lang, one of Languages.
func New(lang string) (*Localizer, error) {
	if !slices.Contains(Languages, lang) {
		return nil, fmt.Errorf("unsupported language %q: use one of %v", lang, Languages)
	}
	bundle, err := newBundle()
	if err != nil {
		return nil, err
	}
	return &Localizer{l: i18n.NewLocalizer(bundle, lang)}, nil
}

func newBundle() (*i18n.Bundle, error) {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	files, err := fs.Glob(messages, "messages/*.toml")
	if err != nil {
		return nil, fmt.Errorf("fs.Glob(): %w", err)
	}
	for _, f := range files {
		b, err := messages.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("embed.FS#ReadFile(): %w", err)
		}
		if _, err := bundle.ParseMessageFileBytes(b, f); err != nil {
			return nil, fmt.Errorf("i18n.Bundle#ParseMessageFileBytes(%s): %w", f, err)
		}
	}
	return bundle, nil
}

// T returns the message id, filled in with data such as map[string]any{"Count": 1}.
// A missing message returns the id, so a typo shows up in Slack instead of an empty text.
func (l *Localizer) T(id string, data ...map[string]any) string {
	cfg := &i18n.LocalizeConfig{MessageID: id}
	if len(data) > 0 {
		cfg.TemplateData = data[0]
	}
	s, err := l.l.Localize(cfg)
	if err != nil {
		return id
	}
	return s
}
