package review

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/monokrome/codereview/internal/diff"
	"github.com/monokrome/codereview/internal/prompt"
	"github.com/monokrome/codereview/internal/provider"
)

type Config struct {
	Diff         string
	Provider     provider.ReviewFunc
	Instructions string
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	files, err := diff.Parse(cfg.Diff)
	if err != nil {
		return Result{}, fmt.Errorf("parsing diff: %w", err)
	}

	system, user := prompt.Build(files, cfg.Instructions)

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

	if err := validateResult(result, files); err != nil {
		return Result{}, fmt.Errorf("validating result: %w", err)
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

func validateResult(result Result, files []diff.File) error {
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

	for i, c := range result.Comments {
		if !IsValidLabel(c.Label) {
			return fmt.Errorf("comment %d: invalid label %q", i, c.Label)
		}

		if c.Line <= 0 {
			return fmt.Errorf("comment %d: line number must be positive, got %d", i, c.Line)
		}

		fileLines, ok := validPaths[c.Path]
		if !ok {
			return fmt.Errorf("comment %d: path %q not found in diff", i, c.Path)
		}

		if !fileLines[c.Line] {
			return fmt.Errorf("comment %d: line %d not found in diff for path %q", i, c.Line, c.Path)
		}
	}

	return nil
}
