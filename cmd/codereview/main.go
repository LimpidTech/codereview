package main

import (
	"context"
	"fmt"
	"os"

	"github.com/monokrome/codereview/internal/action"
	"github.com/monokrome/codereview/internal/diff"
	"github.com/monokrome/codereview/internal/github"
	"github.com/monokrome/codereview/internal/prompt"
	"github.com/monokrome/codereview/internal/provider"
	"github.com/monokrome/codereview/internal/provider/gemini"
	"github.com/monokrome/codereview/internal/review"
)

const defaultBotLogin = "github-actions[bot]"

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

	var providerFn provider.ReviewFunc
	switch cfg.Provider {
	case "gemini":
		if cfg.GeminiAPIKey == "" {
			return fmt.Errorf("INPUT_GEMINI_API_KEY is required for gemini provider")
		}
		providerFn = gemini.New(cfg.GeminiAPIKey, cfg.Model)
	default:
		return fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}

	gh := github.New(cfg.GitHubToken)

	switch cfg.Mode {
	case action.ModeReview:
		return runReview(ctx, cfg, providerFn, gh)
	case action.ModeReply:
		return runReply(ctx, cfg, providerFn, gh)
	default:
		return fmt.Errorf("unknown mode: %s", cfg.Mode)
	}
}

func runReview(ctx context.Context, cfg action.Config, providerFn provider.ReviewFunc, gh *github.Client) error {
	diffText, err := gh.FetchDiff(ctx, cfg.Owner, cfg.Repo, cfg.PRNumber)
	if err != nil {
		return fmt.Errorf("fetching diff: %w", err)
	}

	botLogin := cfg.BotLogin
	if botLogin == "" {
		botLogin = defaultBotLogin
	}

	priorGH, err := gh.FetchBotReviewComments(ctx, cfg.Owner, cfg.Repo, cfg.PRNumber, botLogin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch prior comments: %v\n", err)
	}

	var priorComments []prompt.PriorComment
	for _, pc := range priorGH {
		priorComments = append(priorComments, prompt.PriorComment{
			Path: pc.Path,
			Body: pc.Body,
		})
	}

	fileContents := fetchChangedFiles(ctx, gh, cfg, diffText)

	result, err := review.Run(ctx, review.Config{
		Diff:          diffText,
		Provider:      providerFn,
		Instructions:  cfg.Instructions,
		PriorComments: priorComments,
		FileContents:  fileContents,
	})
	if err != nil {
		return fmt.Errorf("running review: %w", err)
	}

	if err := gh.SubmitReview(ctx, cfg.Owner, cfg.Repo, cfg.PRNumber, cfg.CommitSHA, result); err != nil {
		return fmt.Errorf("submitting review: %w", err)
	}

	fmt.Fprintf(os.Stderr, "review submitted: %s\n", result.Verdict)
	return nil
}

func runReply(ctx context.Context, cfg action.Config, providerFn provider.ReviewFunc, gh *github.Client) error {
	if cfg.SkipReply {
		fmt.Fprintf(os.Stderr, "skipping: comment is from bot or is a top-level comment\n")
		return nil
	}

	thread, err := gh.FetchCommentThread(ctx, cfg.Owner, cfg.Repo, cfg.PRNumber, cfg.Comment.CommentID)
	if err != nil {
		return fmt.Errorf("fetching comment thread: %w", err)
	}

	var messages []prompt.ThreadMessage
	for _, tc := range thread {
		messages = append(messages, prompt.ThreadMessage{
			Author: tc.UserLogin,
			Body:   tc.Body,
		})
	}

	replyText, err := review.RunReply(ctx, review.ReplyConfig{
		Provider:     providerFn,
		Thread:       messages,
		DiffHunk:     cfg.Comment.DiffHunk,
		Instructions: cfg.Instructions,
	})
	if err != nil {
		return fmt.Errorf("generating reply: %w", err)
	}

	replyTo := cfg.Comment.InReplyToID
	if replyTo == 0 {
		replyTo = cfg.Comment.CommentID
	}

	if err := gh.ReplyToComment(ctx, cfg.Owner, cfg.Repo, cfg.PRNumber, replyTo, replyText); err != nil {
		return fmt.Errorf("posting reply: %w", err)
	}

	fmt.Fprintf(os.Stderr, "reply posted\n")
	return nil
}

func fetchChangedFiles(ctx context.Context, gh *github.Client, cfg action.Config, diffText string) map[string]string {
	files, err := diff.Parse(diffText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not parse diff for file fetching: %v\n", err)
		return nil
	}

	contents := make(map[string]string)
	for _, f := range files {
		content, err := gh.FetchFile(ctx, cfg.Owner, cfg.Repo, cfg.CommitSHA, f.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not fetch %s: %v\n", f.Path, err)
			continue
		}

		if content == "" {
			continue
		}

		contents[f.Path] = content
	}

	return contents
}
