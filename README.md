# slack-ai-commentator

[![Build status][github-actions-image]][github-actions-url]

[github-actions-image]: https://github.com/kenchan0130/slack-ai-commentator/workflows/CI/badge.svg
[github-actions-url]: https://github.com/kenchan0130/slack-ai-commentator/actions?query=workflow%3A%22CI%22

A self-hosted Slack app that replies in the thread of Slack posts with what an LLM writes for a fixed prompt.

Invite the app to a channel and add rules there. A rule decides which posts the app replies to and with what prompt, for example:

- Evaluate every post from the RSS app: how the post relates to your company, and whether it is positive or negative.
- When someone reacts with `:mag:`, summarize the post and list the points to check.
- When someone mentions the app in a thread, review the thread's first post.

The app only replies in threads, and escapes the replies so they never mention anyone. It can not do anything else.
A post may still contain instructions that the LLM follows, so treat a reply as untrusted text, and do not write secrets in a prompt: anyone in the channel can see the rules.

## How it works

```
Slack ── post, reaction or mention ──▶ slack-ai-commentator ── prompt + post ──▶ LLM (OpenAI-compatible API)
  ▲                                                                              │
  └──────────────────────── reply in the thread ◀────────────────────────────────┘
```

- The app connects to Slack with [Socket Mode](https://api.slack.com/apis/socket-mode), so it needs no public URL.
- The LLM is any server with an OpenAI-compatible `chat/completions` API, such as [tensai](https://github.com/mattn/tensai), Ollama, llama.cpp or a hosted API.
- Posts are queued and replied to one by one, or `LLM_CONCURRENCY` at a time. The queue is kept in memory, so the posts waiting in it are lost when the app stops. When the queue is full, a post is skipped, and the user who reacted or mentioned is told to try again.
- A channel can have up to 12 rules.

## Rules

Manage the rules of a channel with the slash command of the app, `/ai-commentator` with [manifest.yml](manifest.yml). The app answers whatever command name you give it.

| command | description |
|---------|-------------|
| `/ai-commentator` or `/ai-commentator help` | Show the usage, with buttons to list and add rules. |
| `/ai-commentator list` | Show the rules of the channel, with buttons to edit and delete them. |
| `/ai-commentator add` | Open the dialog to add a rule. |

When the app is invited to a channel without rules, it shows the inviter a button to add one.

| field | description |
|-------|-------------|
| Reply | `When I am mentioned`: the post mentioning the app, or the thread's first post when it is mentioned in a thread. `Every new post in this channel`: top-level posts, not thread replies. `When a post gets a reaction`: posts that get the reaction emoji. |
| Reaction emoji | Shown only for the reaction trigger. Type to search the custom and standard emoji. |
| Posts by | Comma-separated user IDs (`U…`), bot IDs (`B…`) or app names (e.g. `RSS`). Empty for everyone. |
| Prompt | The instructions for the LLM. The post is given after it. |

Anyone who can run the slash command in a channel can change its rules.

## Setup

### 1. Create the Slack app

1. Open https://api.slack.com/apps?new_app=1 and choose "From a manifest".
2. Paste [manifest.yml](manifest.yml) and create the app.
3. Basic Information > App-Level Tokens: generate a token with the `connections:write` scope. This is `SLACK_APP_TOKEN` (`xapp-…`).
4. Install App: install it to the workspace. The Bot User OAuth Token is `SLACK_BOT_TOKEN` (`xoxb-…`).

### 2. Configure the app

The settings that depend on your environment and LLM are given explicitly with environment variables. Only the tuning in [Configuration](#configuration) has defaults.
The settings below are needed whatever LLM you use:

```yaml
    environment:
      SLACK_BOT_TOKEN: xoxb-...
      SLACK_APP_TOKEN: xapp-...
      UI_LANGUAGE: en                       # or ja
      STORE: file:///opt/data/rules.json
      LLM_MAX_TOKENS: "1024"
      LLM_CONCURRENCY: "1"
```

Add the settings of the LLM you use.

#### A local LLM (tensai, Ollama, llama.cpp, ...)

Run an OpenAI-compatible server next to the app and point `LLM_URL` at it. No API key is needed, and posts never leave your network.
An example with [tensai](https://github.com/mattn/tensai), which downloads its model (about 2GB) on the first start and uses about 2.5GB of memory:

```yaml
services:
  slack-ai-commentator:
    image: ghcr.io/kenchan0130/slack-ai-commentator:<version>
    restart: always
    environment:
      # ... the settings above, and:
      LLM_PROVIDER: openai
      LLM_URL: http://tensai:8080/v1
      LLM_MODEL: qwen3
      LLM_CONCURRENCY: "1"  # tensai handles one request at a time
    volumes:
      - data:/opt/data
    depends_on:
      - tensai

  tensai:
    image: ghcr.io/mattn/tensai:<version>
    restart: always
    command: serve -model Qwen/Qwen3-1.7B-GGUF/Qwen3-1.7B-Q8_0.gguf
    volumes:
      - tensai-cache:/cache

volumes:
  data:
  tensai-cache:
```

tensai answers with the model it serves whatever `LLM_MODEL` says. A small model on a CPU takes from a few seconds to tens of seconds for a reply.
For Ollama, set `LLM_URL: http://ollama:11434/v1` and `LLM_MODEL` to the model you pulled.

#### OpenAI

```yaml
    environment:
      # ... the settings above, and:
      LLM_PROVIDER: openai
      LLM_URL: https://api.openai.com/v1
      LLM_MODEL: gpt-4.1-mini
      LLM_API_KEY: sk-...
```

Other services with an OpenAI-compatible API work the same way with their `LLM_URL`, `LLM_MODEL` and `LLM_API_KEY`.

#### Amazon Bedrock

The app calls the [Converse API](https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference.html) with the default AWS credentials, such as the IAM role of an EC2 instance or an ECS task, so no API key is needed.

```yaml
    environment:
      # ... the settings above, and:
      LLM_PROVIDER: bedrock
      LLM_MODEL: <model ID or inference profile ID>
      AWS_BEDROCK_REGION: us-east-1
```

- `AWS_BEDROCK_REGION` is the region of Bedrock only. New models often come to some regions such as `us-east-1` first, so it can differ from the region of your other AWS resources, such as `AWS_SSM_REGION`.
- `LLM_MODEL` is the ID shown in the Bedrock console of the region. Some models can only be called through an inference profile, whose ID starts with a geography such as `apac.` or `us.`, matching the region.
- Enable access to the model in the Bedrock console of the region.
- Allow the role to call the model:

  ```json
  {
    "Effect": "Allow",
    "Action": "bedrock:InvokeModel",
    "Resource": [
      "arn:aws:bedrock:*::foundation-model/*",
      "arn:aws:bedrock:*:<account ID>:inference-profile/*"
    ]
  }
  ```

  Narrow `Resource` down to the models you use.

### 3. Run the app

The image is published at `ghcr.io/kenchan0130/slack-ai-commentator` and `docker.io/kenchan0130/slack-ai-commentator` for `linux/amd64` and `linux/arm64`.
The app stops at start with the names of the settings that are missing.

The app runs as `nobody` (UID 65534). A named volume mounted at `/opt/data` is writable as it is. For a bind mount, make the host directory writable by UID 65534.

### Configuration

| variable | required (default) | description |
|----------|--------------------|-------------|
| `SLACK_BOT_TOKEN` | yes | Bot User OAuth Token. |
| `SLACK_APP_TOKEN` | yes | App-Level Token with `connections:write`. |
| `UI_LANGUAGE` | yes | Language of the messages, buttons and dialogs in Slack: `en` or `ja`. |
| `STORE` | yes | Where the rules are stored: `file://<path>` for a JSON file or `sqlite://<path>` for SQLite, e.g. `file:///opt/data/rules.json`. |
| `LLM_PROVIDER` | yes | `openai` for an OpenAI-compatible API, or `bedrock` for Amazon Bedrock. |
| `LLM_URL` | for `openai` | Base URL of an OpenAI-compatible API, including the version path, e.g. `https://api.openai.com/v1` or `http://tensai:8080/v1`. For Bedrock, it optionally overrides the endpoint, e.g. for a VPC endpoint. |
| `LLM_MODEL` | yes | Model name, or the model ID or inference profile ID for Bedrock. |
| `LLM_API_KEY` | | API key of an OpenAI-compatible API, sent as a bearer token. Not used for local LLMs that need none, nor for Bedrock. |
| `AWS_BEDROCK_REGION` | for `bedrock` | Region of Amazon Bedrock, e.g. `us-east-1`. |
| `AWS_SSM_REGION` | for `ssm://` | Region of the SSM parameters of the `ssm://` secrets, e.g. `ap-northeast-1`. |
| `LLM_MAX_TOKENS` | yes | The most tokens of a reply, e.g. `1024`. |
| `LLM_CONCURRENCY` | yes | Posts replied to at a time, e.g. `1`. Use `1` for a server that handles one request at a time, such as tensai. |
| `LLM_TIMEOUT` | (`5m`) | Timeout of a request to the LLM. |
| `LLM_RETRIES` | (`10`) | Retries of a failed request, with backoff. A local LLM may still be loading its model when the app starts. |
| `QUEUE_SIZE` | (`100`) | Posts waiting for a reply. Posts beyond it are skipped. |
| `MAX_INPUT_CHARS` | (`4000`) | Posts longer than this are truncated before they are given to the LLM. |
| `LOG_FORMAT` | (`text`) | Format of the log: `text` to read, or `json` for log collectors. The log shows the progress of each post, such as queued, generating and replied, with the rule, the channel and the time it took. It never has the text of a post or a reply. |
| `DEBUG` | (`false`) | Logs Slack API calls when set to `true`. |

`SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN` and `LLM_API_KEY` can be read from AWS Systems Manager Parameter Store: set them to `ssm://<parameter name>`, e.g. `ssm:///app/slack_bot_token`, and set `AWS_SSM_REGION`. The default AWS credentials, such as the IAM role of an EC2 instance, are used.

Each AWS service has its own region setting, so `AWS_REGION` is not used for them.

## Development

```sh
make install
make test
make tools lint
```

`go test ./internal/llm -run Integration` calls a real LLM when `LLM_TEST_URL` is set, e.g. `LLM_TEST_URL=http://localhost:8080/v1`.
