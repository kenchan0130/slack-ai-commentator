package bot

import (
	"encoding/json"
	"testing"

	"github.com/slack-go/slack"
)

func TestMessageText(t *testing.T) {
	for name, tc := range map[string]struct {
		raw  string
		want string
	}{
		"plain": {
			raw:  `{"type":"message","text":"Tom &amp; Jerry &lt;3"}`,
			want: "Tom & Jerry <3",
		},
		"rss app attachment": {
			raw: `{"type":"message","subtype":"bot_message","bot_id":"B1","text":"",
				"attachments":[{"fallback":"fb","title":"サンプルバーガーの新作がおいしくなかった","title_link":"https://x.com/a/status/1","text":"https://x.com/a/status/1"}]}`,
			want: "サンプルバーガーの新作がおいしくなかった\nhttps://x.com/a/status/1",
		},
		"fallback only": {
			raw:  `{"type":"message","attachments":[{"fallback":"only fallback"}]}`,
			want: "only fallback",
		},
		"blocks only": {
			raw: `{"type":"message","blocks":[
				{"type":"header","text":{"type":"plain_text","text":"Title"}},
				{"type":"section","text":{"type":"mrkdwn","text":"Body"},"fields":[{"type":"mrkdwn","text":"F1"}]},
				{"type":"context","elements":[{"type":"mrkdwn","text":"ctx"}]}]}`,
			want: "Title\nBody\nF1\nctx",
		},
		"summary in text and details in blocks": {
			raw:  `{"type":"message","text":"New alert","blocks":[{"type":"section","text":{"type":"mrkdwn","text":"New alert"}},{"type":"section","text":{"type":"mrkdwn","text":"CPU is 95% on web-1"}}]}`,
			want: "New alert\nCPU is 95% on web-1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var m slack.Msg
			if err := json.Unmarshal([]byte(tc.raw), &m); err != nil {
				t.Fatal(err)
			}
			if got := messageText(m); got != tc.want {
				t.Errorf("messageText() = %q, want %q", got, tc.want)
			}
		})
	}
}
