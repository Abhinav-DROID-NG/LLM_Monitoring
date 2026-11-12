package provider

import "context"

type Provider interface {
	ChatCompletion(context.Context, Request) (Response, error)
}

type Request struct {
	Model          string
	Messages       []Message
	Temperature    *float64
	MaxTokens      *int
	ResponseFormat string
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Response struct {
	ID                string
	Model             string
	SystemFingerprint string
	Choices           []Choice
	Usage             Usage
}

type Choice struct {
	Message      Message
	FinishReason string
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}
