package provider

import "context"

type Request struct {
	SystemPrompt string
	UserPrompt   string
}

type Response struct {
	Content string
}

type ReviewFunc func(ctx context.Context, req Request) (Response, error)
