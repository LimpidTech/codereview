package main

import (
	"context"
	"fmt"
	"os"

	"github.com/monokrome/codereview/internal/action"
	"github.com/monokrome/codereview/internal/github"
	"github.com/monokrome/codereview/internal/provider/gemini"
	"github.com/monokrome/codereview/internal/review"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := action.Parse()
	if err != nil {
		return fmt.Errorf("parsing inputs: %w", err)
	}

	var reviewFn review.Config
	switch cfg.Provider {
	case "gemini":
		if cfg.GeminiAPIKey == "" {
			return fmt.Errorf("INPUT_GEMINI_API_KEY is required for gemini provider")
		}
		reviewFn.Provider = gemini.New(cfg.GeminiAPIKey, cfg.Model)
	default:
		return fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}

	gh := github.New(cfg.GitHubToken)

	diffText, err := gh.FetchDiff(ctx, cfg.Owner, cfg.Repo, cfg.PRNumber)
	if err != nil {
		return fmt.Errorf("fetching diff: %w", err)
	}

	reviewFn.Diff = diffText
	reviewFn.Instructions = cfg.Instructions

	result, err := review.Run(ctx, reviewFn)
	if err != nil {
		return fmt.Errorf("running review: %w", err)
	}

	if err := gh.SubmitReview(ctx, cfg.Owner, cfg.Repo, cfg.PRNumber, cfg.CommitSHA, result); err != nil {
		return fmt.Errorf("submitting review: %w", err)
	}

	fmt.Fprintf(os.Stderr, "review submitted: %s\n", result.Verdict)
	return nil
}
