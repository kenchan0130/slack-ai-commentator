package locale

import (
	"slices"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestMessagesHaveTheSameKeys(t *testing.T) {
	keys := map[string][]string{}
	for _, lang := range Languages {
		b, err := messages.ReadFile("messages/" + lang + ".toml")
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]string
		if err := toml.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		for k := range m {
			keys[lang] = append(keys[lang], k)
		}
		slices.Sort(keys[lang])
	}
	for _, lang := range Languages[1:] {
		if !slices.Equal(keys[lang], keys[Languages[0]]) {
			t.Errorf("keys of %s differ from %s:\n%v\n%v", lang, Languages[0], keys[lang], keys[Languages[0]])
		}
	}
}

func TestT(t *testing.T) {
	ja, err := New("ja")
	if err != nil {
		t.Fatal(err)
	}
	if got := ja.T("ListHeader", map[string]any{"Channel": "C1", "Count": 2}); got != "<#C1> のルール: 2件" {
		t.Errorf("T(ListHeader) = %q", got)
	}
	en, err := New("en")
	if err != nil {
		t.Fatal(err)
	}
	if got := en.T("Save"); got != "Save" {
		t.Errorf("T(Save) = %q", got)
	}
	if got := en.T("NoSuchMessage"); got != "NoSuchMessage" {
		t.Errorf("T(NoSuchMessage) = %q, want the id", got)
	}
	if _, err := New("fr"); err == nil {
		t.Error("New(fr) succeeded")
	}
}
