package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/monokrome/codereview/internal/review"
)

const apiBase = "https://api.github.com"

type Client struct {
	token      string
	httpClient *http.Client
}

func New(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{},
	}
}

func (c *Client) FetchDiff(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", apiBase, owner, repo, prNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3.diff")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching diff: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	return string(data), nil
}

type reviewComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Body string `json:"body"`
	Side string `json:"side"`
}

type reviewRequest struct {
	Event    string          `json:"event"`
	Body     string          `json:"body"`
	Comments []reviewComment `json:"comments"`
	CommitID string          `json:"commit_id"`
}

func (c *Client) SubmitReview(ctx context.Context, owner, repo string, prNumber int, commitSHA string, result review.Result) error {
	comments := make([]reviewComment, len(result.Comments))
	for i, rc := range result.Comments {
		comments[i] = reviewComment{
			Path: rc.Path,
			Line: rc.Line,
			Body: rc.Body,
			Side: "RIGHT",
		}
	}

	payload := reviewRequest{
		Event:    string(result.Verdict),
		Body:     result.Summary,
		Comments: comments,
		CommitID: commitSHA,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling review: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews", apiBase, owner, repo, prNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("submitting review: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

type ThreadComment struct {
	ID          int64  `json:"id"`
	InReplyToID int64  `json:"in_reply_to_id"`
	Body        string `json:"body"`
	UserLogin   string
	Path        string `json:"path"`
	Line        int    `json:"line"`
	DiffHunk    string `json:"diff_hunk"`
	CreatedAt   string `json:"created_at"`
}

func (c *Client) FetchCommentThread(ctx context.Context, owner, repo string, prNumber int, commentID int64) ([]ThreadComment, error) {
	var allComments []struct {
		ID          int64  `json:"id"`
		InReplyToID int64  `json:"in_reply_to_id"`
		Body        string `json:"body"`
		Path        string `json:"path"`
		Line        int    `json:"line"`
		DiffHunk    string `json:"diff_hunk"`
		CreatedAt   string `json:"created_at"`
		User        struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	page := 1
	for {
		url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments?per_page=100&page=%d", apiBase, owner, repo, prNumber, page)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching comments: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
		}

		var pageComments []struct {
			ID          int64  `json:"id"`
			InReplyToID int64  `json:"in_reply_to_id"`
			Body        string `json:"body"`
			Path        string `json:"path"`
			Line        int    `json:"line"`
			DiffHunk    string `json:"diff_hunk"`
			CreatedAt   string `json:"created_at"`
			User        struct {
				Login string `json:"login"`
			} `json:"user"`
		}

		if err := json.Unmarshal(body, &pageComments); err != nil {
			return nil, fmt.Errorf("parsing comments: %w", err)
		}

		allComments = append(allComments, pageComments...)

		if len(pageComments) < 100 {
			break
		}
		page++
	}

	rootID := commentID
	commentsByID := make(map[int64]struct {
		ID          int64  `json:"id"`
		InReplyToID int64  `json:"in_reply_to_id"`
		Body        string `json:"body"`
		Path        string `json:"path"`
		Line        int    `json:"line"`
		DiffHunk    string `json:"diff_hunk"`
		CreatedAt   string `json:"created_at"`
		User        struct {
			Login string `json:"login"`
		} `json:"user"`
	})
	for _, c := range allComments {
		commentsByID[c.ID] = c
	}

	if target, ok := commentsByID[commentID]; ok && target.InReplyToID != 0 {
		rootID = target.InReplyToID
	}

	var thread []ThreadComment
	for _, c := range allComments {
		if c.ID == rootID || c.InReplyToID == rootID {
			thread = append(thread, ThreadComment{
				ID:          c.ID,
				InReplyToID: c.InReplyToID,
				Body:        c.Body,
				UserLogin:   c.User.Login,
				Path:        c.Path,
				Line:        c.Line,
				DiffHunk:    c.DiffHunk,
				CreatedAt:   c.CreatedAt,
			})
		}
	}

	return thread, nil
}

type replyRequest struct {
	Body      string `json:"body"`
	InReplyTo int64  `json:"in_reply_to"`
}

func (c *Client) ReplyToComment(ctx context.Context, owner, repo string, prNumber int, inReplyTo int64, body string) error {
	payload := replyRequest{
		Body:      body,
		InReplyTo: inReplyTo,
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling reply: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments", apiBase, owner, repo, prNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(reqBody)))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("posting reply: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
