# codereview

LLM-powered pull request code review as a GitHub Action. Posts inline review comments using [conventional comments](https://conventionalcomments.org/) format and submits a verdict (approve, request changes, or comment).

## Features

- Reviews PR diffs with full file context
- Uses conventional comment labels: `nit:`, `suggestion:`, `issue:`, `question:`, `thought:`, `chore:`, `praise:`
- Replies to comment threads when developers respond
- Resolves threads automatically when concerns are addressed
- Remembers prior comments to avoid repeating itself
- Configurable LLM provider (Gemini supported, more planned)

## Quick start

Add a workflow file to your repository:

```yaml
# .github/workflows/review.yml
name: Code Review

on:
  pull_request:
    types: [opened, synchronize]
  pull_request_review_comment:
    types: [created]

permissions:
  pull-requests: write
  contents: read

jobs:
  review:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: LimpidTech/codereview@main
        with:
          gemini-api-key: ${{ secrets.GEMINI_API_KEY }}
```

Get a Gemini API key from [Google AI Studio](https://aistudio.google.com/apikey) and add it as a repository or organization secret named `GEMINI_API_KEY`.

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `github-token` | no | `${{ github.token }}` | GitHub token for API access |
| `provider` | no | `gemini` | LLM provider to use |
| `gemini-api-key` | no | | API key for Google Gemini |
| `model` | no | `gemini-2.5-flash` | Model name override |
| `instructions` | no | | Additional review instructions |
| `bot-login` | no | `github-actions[bot]` | Bot username for loop prevention |

## Providers

### Gemini (default)

```yaml
- uses: LimpidTech/codereview@main
  with:
    provider: gemini
    gemini-api-key: ${{ secrets.GEMINI_API_KEY }}
```

To use a specific model:

```yaml
- uses: LimpidTech/codereview@main
  with:
    gemini-api-key: ${{ secrets.GEMINI_API_KEY }}
    model: gemini-2.5-pro
```

### Adding providers

The codebase is designed for easy provider addition. Each provider implements a single function type:

```go
type ReviewFunc func(ctx context.Context, req Request) (Response, error)
```

See `internal/provider/gemini/` for a reference implementation.

## Custom instructions

Use the `instructions` input to guide the reviewer toward your project's conventions:

```yaml
- uses: LimpidTech/codereview@main
  with:
    gemini-api-key: ${{ secrets.GEMINI_API_KEY }}
    instructions: |
      This is a Go project using the standard library only.
      We prefer table-driven tests.
      Error messages should not be capitalized.
      All exported functions must have doc comments.
```

Instructions are prepended to the review prompt, so the LLM will consider them alongside the diff and file context.

## How it works

### On PR open / push

1. Fetches the PR diff and full contents of changed files
2. Fetches any prior review comments it has already made
3. Sends everything to the configured LLM with review instructions
4. Posts inline review comments with conventional labels
5. Submits a verdict: `APPROVE`, `REQUEST_CHANGES`, or `COMMENT`

### On review comment reply

1. Fetches the full conversation thread
2. Sends the thread context and relevant diff hunk to the LLM
3. Posts a reply
4. If the concern is resolved, automatically resolves the thread (only for threads it started)

## Permissions

The workflow needs these permissions:

```yaml
permissions:
  pull-requests: write  # post reviews, reply to comments, resolve threads
  contents: read        # fetch file contents for context
```

## Using with a GitHub App token

If you use a GitHub App token instead of the default `GITHUB_TOKEN`, set `bot-login` to the app's bot username so it doesn't reply to its own comments:

```yaml
- uses: LimpidTech/codereview@main
  with:
    github-token: ${{ steps.app-token.outputs.token }}
    gemini-api-key: ${{ secrets.GEMINI_API_KEY }}
    bot-login: my-app[bot]
```

## License

BSD 2-Clause. See [LICENSE](LICENSE) for details.
