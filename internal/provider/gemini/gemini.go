package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/monokrome/codereview/internal/provider"
)

const (
	DefaultModel = "gemini-2.5-flash"
	apiBaseURL   = "https://generativelanguage.googleapis.com/v1beta/models"
)

type part struct {
	Text string `json:"text"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type generateRequest struct {
	Contents          []content `json:"contents"`
	SystemInstruction *content  `json:"systemInstruction,omitempty"`
}

type candidate struct {
	Content content `json:"content"`
}

type generateResponse struct {
	Candidates []candidate `json:"candidates"`
}

func New(apiKey string, model string) provider.ReviewFunc {
	if model == "" {
		model = DefaultModel
	}

	return func(ctx context.Context, req provider.Request) (provider.Response, error) {
		payload := generateRequest{
			Contents: []content{
				{Role: "user", Parts: []part{{Text: req.UserPrompt}}},
			},
		}

		if req.SystemPrompt != "" {
			payload.SystemInstruction = &content{
				Parts: []part{{Text: req.SystemPrompt}},
			}
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return provider.Response{}, fmt.Errorf("marshalling request: %w", err)
		}

		url := fmt.Sprintf("%s/%s:generateContent?key=%s", apiBaseURL, model, apiKey)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
		if err != nil {
			return provider.Response{}, fmt.Errorf("creating request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			return provider.Response{}, fmt.Errorf("calling Gemini API: %w", err)
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return provider.Response{}, fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return provider.Response{}, fmt.Errorf("Gemini API returned status %d: %s", resp.StatusCode, string(respBody))
		}

		var genResp generateResponse
		if err := json.Unmarshal(respBody, &genResp); err != nil {
			return provider.Response{}, fmt.Errorf("parsing response: %w", err)
		}

		if len(genResp.Candidates) == 0 {
			return provider.Response{}, fmt.Errorf("no candidates in response")
		}

		parts := genResp.Candidates[0].Content.Parts
		if len(parts) == 0 {
			return provider.Response{}, fmt.Errorf("no parts in response candidate")
		}

		return provider.Response{Content: parts[0].Text}, nil
	}
}
