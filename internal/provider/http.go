package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type HTTPClient struct {
	baseURL string
	client  *http.Client
	retries int
	sleep   func(context.Context, time.Duration) error
}

func NewHTTPClient(baseURL string, client *http.Client, retries int) (*HTTPClient, error) {
	if baseURL == "" {
		return nil, errors.New("provider URL cannot be empty")
	}
	if client == nil {
		return nil, errors.New("HTTP client cannot be nil")
	}
	if retries < 0 {
		return nil, errors.New("provider retries cannot be negative")
	}
	return &HTTPClient{
		baseURL: baseURL,
		client:  client,
		retries: retries,
		sleep:   sleepContext,
	}, nil
}

func (c *HTTPClient) ChatCompletion(ctx context.Context, request Request) (Response, error) {
	payload := struct {
		Model          string    `json:"model"`
		Messages       []Message `json:"messages"`
		Temperature    *float64  `json:"temperature,omitempty"`
		MaxTokens      *int      `json:"max_tokens,omitempty"`
		ResponseFormat string    `json:"response_format,omitempty"`
	}{
		Model: request.Model, Messages: request.Messages, Temperature: request.Temperature,
		MaxTokens: request.MaxTokens, ResponseFormat: request.ResponseFormat,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, fmt.Errorf("marshal provider request: %w", err)
	}

	for attempt := 0; attempt <= c.retries; attempt++ {
		response, retryAfter, requestErr := c.do(ctx, body)
		if requestErr == nil {
			return response, nil
		}
		if attempt == c.retries || !isRetryable(requestErr) {
			return Response{}, requestErr
		}
		if err := c.sleep(ctx, retryAfter); err != nil {
			return Response{}, fmt.Errorf("wait before provider retry: %w", err)
		}
	}
	return Response{}, errors.New("provider request exhausted retries")
}

func (c *HTTPClient) do(ctx context.Context, body []byte) (Response, time.Duration, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, 0, fmt.Errorf("create provider request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return Response{}, 0, fmt.Errorf("provider request: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return Response{}, 0, fmt.Errorf("read provider response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		retryAfter := retryDelay(response.Header.Get("Retry-After"))
		requestErr := fmt.Errorf("provider returned HTTP %d: %s", response.StatusCode, string(responseBody))
		if response.StatusCode == http.StatusTooManyRequests {
			return Response{}, retryAfter, rateLimitError{err: requestErr}
		}
		if response.StatusCode >= 500 {
			return Response{}, retryAfter, serverError{err: requestErr}
		}
		return Response{}, 0, requestErr
	}

	var payload struct {
		ID                string `json:"id"`
		Model             string `json:"model"`
		SystemFingerprint string `json:"system_fingerprint"`
		Choices           []struct {
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return Response{}, 0, fmt.Errorf("decode provider response: %w", err)
	}
	result := Response{ID: payload.ID, Model: payload.Model, SystemFingerprint: payload.SystemFingerprint, Usage: payload.Usage}
	for _, choice := range payload.Choices {
		result.Choices = append(result.Choices, Choice{Message: choice.Message, FinishReason: choice.FinishReason})
	}
	return result, 0, nil
}

type rateLimitError struct{ err error }

func (e rateLimitError) Error() string { return e.err.Error() }
func (e rateLimitError) Unwrap() error { return e.err }

type serverError struct{ err error }

func (e serverError) Error() string { return e.err.Error() }
func (e serverError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var rateErr rateLimitError
	var serverErr serverError
	return errors.As(err, &rateErr) || errors.As(err, &serverErr)
}

func retryDelay(value string) time.Duration {
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return 100 * time.Millisecond
	}
	return time.Duration(seconds) * time.Second
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
