package prompt

import (
	"fmt"
	"strings"

	"github.com/monokrome/codereview/internal/diff"
)

const systemTemplate = `You are a senior code reviewer. Review the provided unified diff and produce a JSON response.

Use these conventional comment labels:
- "nit": style or trivial improvements that don't affect correctness
- "suggestion": a better approach or alternative worth considering
- "issue": a bug, logic error, or correctness problem that must be fixed
- "question": something unclear that needs the author's explanation
- "thought": an observation or design consideration, not actionable
- "chore": maintenance tasks like updating dependencies, fixing typos, or cleanup
- "praise": something done well that deserves recognition

Verdict rules:
- Use "APPROVE" when there are no issues or only nits/praise/thoughts
- Use "REQUEST_CHANGES" when there are any comments with the "issue" label
- Use "COMMENT" for everything else (suggestions, questions, chores without issues)

Your response must be valid JSON matching this exact structure:
{
  "verdict": "APPROVE" | "REQUEST_CHANGES" | "COMMENT",
  "summary": "brief overall summary of the review",
  "comments": [
    {
      "path": "file/path.go",
      "line": 42,
      "label": "issue",
      "body": "issue: description of the problem"
    }
  ]
}

Rules:
- The "line" field must reference a valid line number from the diff (a line that was added or exists as context in the new file)
- The "body" field must start with the label followed by a colon and space (e.g. "nit: unused import")
- The "path" field must match a file path from the diff exactly
- Only comment on meaningful changes; do not comment on every line
- Be concise and actionable
- Output ONLY the JSON object, no markdown fences, no extra text`

const replySystemTemplate = `You are a senior code reviewer engaged in a conversation about a code review comment. You are replying to a developer who responded to one of your review comments.

Rules:
- Be concise and directly address the question or comment
- Reference the code when relevant
- If the developer's response resolves your concern, acknowledge it
- If you still have concerns, explain why clearly
- Be collaborative, not adversarial

Your response must be valid JSON matching this exact structure:
{
  "reply": "your reply text here",
  "resolved": true
}

- Set "resolved" to true if the developer's response adequately addresses your original concern
- Set "resolved" to false if the concern is not yet addressed or you have follow-up questions
- Output ONLY the JSON object, no markdown fences, no extra text`

type PriorComment struct {
	Path string
	Body string
}

func Build(files []diff.File, instructions string, priorComments []PriorComment, fileContents map[string]string) (string, string) {
	var user strings.Builder

	if instructions != "" {
		fmt.Fprintf(&user, "Additional review instructions:\n%s\n\n", instructions)
	}

	if len(priorComments) > 0 {
		user.WriteString("You have already made the following review comments on this PR. Do NOT repeat these. Only raise new issues or comment on changes that address (or fail to address) your prior feedback:\n\n")
		for _, pc := range priorComments {
			fmt.Fprintf(&user, "- [%s] %s\n", pc.Path, pc.Body)
		}
		user.WriteString("\n")
	}

	if len(fileContents) > 0 {
		user.WriteString("Full file contents for context (use these to understand the surrounding code):\n\n")
		for path, content := range fileContents {
			fmt.Fprintf(&user, "=== %s ===\n%s\n\n", path, content)
		}
	}

	user.WriteString("Review the following diff:\n\n")

	for _, f := range files {
		fmt.Fprintf(&user, "--- a/%s\n+++ b/%s\n", f.Path, f.Path)

		for _, h := range f.Hunks {
			fmt.Fprintf(&user, "@@ -%d +%d @@\n", h.StartLine, h.StartLine)

			for _, l := range h.Lines {
				switch l.Kind {
				case diff.KindAdded:
					fmt.Fprintf(&user, "+%s\n", l.Content)
				case diff.KindRemoved:
					fmt.Fprintf(&user, "-%s\n", l.Content)
				default:
					fmt.Fprintf(&user, " %s\n", l.Content)
				}
			}
		}
	}

	return systemTemplate, user.String()
}

type ThreadMessage struct {
	Author string
	Body   string
}

func BuildReply(thread []ThreadMessage, diffHunk string, instructions string) (string, string) {
	var user strings.Builder

	if instructions != "" {
		fmt.Fprintf(&user, "Additional context:\n%s\n\n", instructions)
	}

	fmt.Fprintf(&user, "Relevant code:\n```\n%s\n```\n\n", diffHunk)

	user.WriteString("Conversation thread:\n\n")
	for _, msg := range thread {
		fmt.Fprintf(&user, "**%s:**\n%s\n\n", msg.Author, msg.Body)
	}

	user.WriteString("Write your reply to the latest message.")

	return replySystemTemplate, user.String()
}
