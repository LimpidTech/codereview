package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/monokrome/codereview/internal/review"
)

const httpTimeout = 30 * time.Second

const apiBase = "https://api.github.com"

type Client struct {
	token      string
	httpClient *http.Client
}

func New(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: httpTimeout},
	}
}

type apiComment struct {
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

func (c *Client) fetchAllPRComments(ctx context.Context, owner, repo string, prNumber int) ([]apiComment, error) {
	var all []apiComment

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
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
		}

		var pageComments []apiComment
		if err := json.Unmarshal(body, &pageComments); err != nil {
			return nil, fmt.Errorf("parsing comments: %w", err)
		}

		all = append(all, pageComments...)

		if len(pageComments) < 100 {
			break
		}
		page++
	}

	return all, nil
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

const maxFileSize = 256 * 1024 // 256KB

func (c *Client) FetchFile(ctx context.Context, owner, repo, ref, path string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", apiBase, owner, repo, path, ref)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3.raw")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFileSize+1))
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	if len(data) > maxFileSize {
		return "", nil
	}

	return string(data), nil
}

type submitComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Body string `json:"body"`
	Side string `json:"side"`
}

type submitRequest struct {
	Event    string          `json:"event"`
	Body     string          `json:"body"`
	Comments []submitComment `json:"comments"`
	CommitID string          `json:"commit_id"`
}

func (c *Client) SubmitReview(ctx context.Context, owner, repo string, prNumber int, commitSHA string, result review.Result) error {
	comments := make([]submitComment, len(result.Comments))
	for i, rc := range result.Comments {
		comments[i] = submitComment{
			Path: rc.Path,
			Line: rc.Line,
			Body: rc.Body,
			Side: "RIGHT",
		}
	}

	payload := submitRequest{
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
	ID          int64
	InReplyToID int64
	Body        string
	UserLogin   string
	Path        string
	Line        int
	DiffHunk    string
	CreatedAt   string
}

func toThreadComment(c apiComment) ThreadComment {
	return ThreadComment{
		ID:          c.ID,
		InReplyToID: c.InReplyToID,
		Body:        c.Body,
		UserLogin:   c.User.Login,
		Path:        c.Path,
		Line:        c.Line,
		DiffHunk:    c.DiffHunk,
		CreatedAt:   c.CreatedAt,
	}
}

func (c *Client) FetchCommentThread(ctx context.Context, owner, repo string, prNumber int, commentID int64) ([]ThreadComment, error) {
	allComments, err := c.fetchAllPRComments(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	commentsByID := make(map[int64]apiComment)
	for _, ac := range allComments {
		commentsByID[ac.ID] = ac
	}

	rootID := commentID
	if target, ok := commentsByID[commentID]; ok && target.InReplyToID != 0 {
		rootID = target.InReplyToID
	}

	var thread []ThreadComment
	for _, ac := range allComments {
		if ac.ID == rootID || ac.InReplyToID == rootID {
			thread = append(thread, toThreadComment(ac))
		}
	}

	return thread, nil
}

type PriorComment struct {
	Path string
	Line int
	Body string
}

func (c *Client) FetchBotReviewComments(ctx context.Context, owner, repo string, prNumber int, botLogin string) ([]PriorComment, error) {
	allComments, err := c.fetchAllPRComments(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	var prior []PriorComment
	for _, ac := range allComments {
		if ac.User.Login == botLogin {
			prior = append(prior, PriorComment{
				Path: ac.Path,
				Line: ac.Line,
				Body: ac.Body,
			})
		}
	}

	return prior, nil
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

const graphqlEndpoint = "https://api.github.com/graphql"

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) ResolveThread(ctx context.Context, owner, repo string, prNumber int, rootCommentID int64, botLogin string) error {
	threadID, isBotThread, err := c.findThreadByComment(ctx, owner, repo, prNumber, rootCommentID, botLogin)
	if err != nil {
		return fmt.Errorf("finding thread: %w", err)
	}

	if !isBotThread {
		return nil
	}

	if threadID == "" {
		return fmt.Errorf("thread not found for comment %d", rootCommentID)
	}

	mutation := `mutation($threadId: ID!) {
		resolveReviewThread(input: { threadId: $threadId }) {
			thread { id isResolved }
		}
	}`

	return c.graphql(ctx, mutation, map[string]any{"threadId": threadID})
}

func (c *Client) findThreadByComment(ctx context.Context, owner, repo string, prNumber int, commentID int64, botLogin string) (string, bool, error) {
	query := `query($owner: String!, $repo: String!, $prNumber: Int!, $cursor: String) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $prNumber) {
				reviewThreads(first: 100, after: $cursor) {
					pageInfo { hasNextPage endCursor }
					nodes {
						id
						comments(first: 1) {
							nodes { databaseId author { login } }
						}
					}
				}
			}
		}
	}`

	vars := map[string]any{
		"owner":    owner,
		"repo":     repo,
		"prNumber": prNumber,
		"cursor":   nil,
	}

	for {
		body, err := c.graphqlRaw(ctx, query, vars)
		if err != nil {
			return "", false, err
		}

		var result struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID       string `json:"id"`
							Comments struct {
								Nodes []struct {
									DatabaseID int64 `json:"databaseId"`
									Author     struct {
										Login string `json:"login"`
									} `json:"author"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return "", false, fmt.Errorf("parsing response: %w", err)
		}

		threads := result.Repository.PullRequest.ReviewThreads
		for _, thread := range threads.Nodes {
			if len(thread.Comments.Nodes) == 0 {
				continue
			}

			firstComment := thread.Comments.Nodes[0]
			if firstComment.DatabaseID == commentID {
				isBotThread := firstComment.Author.Login == botLogin
				return thread.ID, isBotThread, nil
			}
		}

		if !threads.PageInfo.HasNextPage {
			break
		}
		vars["cursor"] = threads.PageInfo.EndCursor
	}

	return "", false, nil
}

func (c *Client) graphql(ctx context.Context, query string, vars map[string]any) error {
	_, err := c.graphqlRaw(ctx, query, vars)
	return err
}

func (c *Client) graphqlRaw(ctx context.Context, query string, vars map[string]any) (json.RawMessage, error) {
	payload := graphqlRequest{Query: query, Variables: vars}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphqlEndpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var gqlResp graphqlResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", gqlResp.Errors[0].Message)
	}

	return gqlResp.Data, nil
}
