package action

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	GitHubToken  string
	Provider     string
	GeminiAPIKey string
	Model        string
	Instructions string
	Owner        string
	Repo         string
	PRNumber     int
	CommitSHA    string
}

type githubEvent struct {
	PullRequest struct {
		Number int `json:"number"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	} `json:"repository"`
}

func Parse() (Config, error) {
	cfg := Config{
		GitHubToken:  os.Getenv("INPUT_GITHUB_TOKEN"),
		Provider:     os.Getenv("INPUT_PROVIDER"),
		GeminiAPIKey: os.Getenv("INPUT_GEMINI_API_KEY"),
		Model:        os.Getenv("INPUT_MODEL"),
		Instructions: os.Getenv("INPUT_INSTRUCTIONS"),
	}

	if cfg.GitHubToken == "" {
		return Config{}, fmt.Errorf("INPUT_GITHUB_TOKEN is required")
	}

	if cfg.Provider == "" {
		cfg.Provider = "gemini"
	}

	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return Config{}, fmt.Errorf("GITHUB_EVENT_PATH is required")
	}

	data, err := os.ReadFile(eventPath)
	if err != nil {
		return Config{}, fmt.Errorf("reading event file: %w", err)
	}

	var event githubEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return Config{}, fmt.Errorf("parsing event JSON: %w", err)
	}

	cfg.Owner = event.Repository.Owner.Login
	cfg.Repo = event.Repository.Name
	cfg.PRNumber = event.PullRequest.Number
	cfg.CommitSHA = event.PullRequest.Head.SHA

	if override := os.Getenv("INPUT_PR_NUMBER"); override != "" {
		n, err := strconv.Atoi(override)
		if err != nil {
			return Config{}, fmt.Errorf("parsing INPUT_PR_NUMBER: %w", err)
		}
		cfg.PRNumber = n
	}

	if cfg.Owner == "" {
		return Config{}, fmt.Errorf("repository owner not found in event")
	}

	if cfg.Repo == "" {
		return Config{}, fmt.Errorf("repository name not found in event")
	}

	if cfg.PRNumber == 0 {
		return Config{}, fmt.Errorf("pull request number not found in event")
	}

	if cfg.CommitSHA == "" {
		return Config{}, fmt.Errorf("commit SHA not found in event")
	}

	return cfg, nil
}
