package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/monokrome/codereview/internal/diff"
	"github.com/monokrome/codereview/internal/prompt"
	"github.com/monokrome/codereview/internal/provider"
)

type Config struct {
	Diff           string
	Provider       provider.ReviewFunc
	Instructions   string
	PriorComments  []prompt.PriorComment
	FileContents   map[string]string
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	files, err := diff.Parse(cfg.Diff)
	if err != nil {
		return Result{}, fmt.Errorf("parsing diff: %w", err)
	}

	system, user := prompt.Build(files, cfg.Instructions, cfg.PriorComments, cfg.FileContents)

	resp, err := cfg.Provider(ctx, provider.Request{
		SystemPrompt: system,
		UserPrompt:   user,
	})
	if err != nil {
		return Result{}, fmt.Errorf("calling provider: %w", err)
	}

	result, err := ParseResponse(resp.Content)
	if err != nil {
		return Result{}, fmt.Errorf("parsing response: %w", err)
	}

	result.Comments = filterValidComments(result.Comments, files)

	return result, nil
}

type ReplyConfig struct {
	Provider     provider.ReviewFunc
	Thread       []prompt.ThreadMessage
	DiffHunk     string
	Instructions string
}

type ReplyResult struct {
	Reply    string `json:"reply"`
	Resolved bool   `json:"resolved"`
}

func RunReply(ctx context.Context, cfg ReplyConfig) (ReplyResult, error) {
	system, user := prompt.BuildReply(cfg.Thread, cfg.DiffHunk, cfg.Instructions)

	resp, err := cfg.Provider(ctx, provider.Request{
		SystemPrompt: system,
		UserPrompt:   user,
	})
	if err != nil {
		return ReplyResult{}, fmt.Errorf("calling provider: %w", err)
	}

	cleaned := strings.TrimSpace(stripMarkdownFences(resp.Content))

	var result ReplyResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return ReplyResult{Reply: cleaned}, nil
	}

	return result, nil
}

func ParseResponse(raw string) (Result, error) {
	cleaned := stripMarkdownFences(raw)
	cleaned = strings.TrimSpace(cleaned)

	var result Result
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return Result{}, fmt.Errorf("unmarshalling JSON: %w", err)
	}

	return result, nil
}

func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)

	if !strings.HasPrefix(s, "```") {
		return s
	}

	firstNewline := strings.Index(s, "\n")
	if firstNewline < 0 {
		return s
	}
	s = s[firstNewline+1:]

	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}

	return strings.TrimSpace(s)
}

func filterValidComments(comments []Comment, files []diff.File) []Comment {
	validPaths := make(map[string]map[int]bool)
	for _, f := range files {
		lines := make(map[int]bool)
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				if l.Kind != diff.KindRemoved {
					lines[l.Number] = true
				}
			}
		}
		validPaths[f.Path] = lines
	}

	var valid []Comment
	for _, c := range comments {
		if !IsValidLabel(c.Label) {
			fmt.Fprintf(os.Stderr, "warning: dropping comment with invalid label %q\n", c.Label)
			continue
		}

		if c.Line <= 0 {
			fmt.Fprintf(os.Stderr, "warning: dropping comment with non-positive line %d\n", c.Line)
			continue
		}

		fileLines, ok := validPaths[c.Path]
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: dropping comment for path %q not in diff\n", c.Path)
			continue
		}

		if !fileLines[c.Line] {
			fmt.Fprintf(os.Stderr, "warning: dropping comment for line %d not in diff for %q\n", c.Line, c.Path)
			continue
		}

		valid = append(valid, c)
	}

	return valid
}
