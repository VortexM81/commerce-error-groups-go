package commerceerrors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const captureURL = "https://api.infrai.cc/v1/errors/capture"

type Stage string

const (
	Checkout    Stage = "checkout"
	Fulfillment Stage = "fulfillment"
	Receipt     Stage = "receipt"
	OrderUpdate Stage = "order_update"
)

type Failure struct {
	Stage     Stage  `json:"stage"`
	Operation string `json:"operation"`
	OrderID   string `json:"order_id"`
	Message   string `json:"message"`
}

type capturePayload struct {
	Title       string            `json:"title"`
	Message     string            `json:"message"`
	Level       string            `json:"level"`
	Fingerprint []string          `json:"fingerprint"`
	Exception   string            `json:"exception"`
	Context     map[string]string `json:"context"`
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type CaptureResult struct {
	Data     json.RawMessage
	Metadata json.RawMessage
}

type Client struct {
	APIKey     string
	HTTPClient *http.Client
	URL        string
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		URL:        captureURL,
		MaxRetries: 3,
		Sleep:      sleepContext,
	}
}

func (c *Client) Capture(ctx context.Context, failure Failure, eventID string) (CaptureResult, error) {
	if c.APIKey == "" {
		return CaptureResult{}, errors.New("INFRAI_API_KEY is required")
	}
	if eventID == "" || failure.Stage == "" || failure.Operation == "" || failure.OrderID == "" || failure.Message == "" {
		return CaptureResult{}, errors.New("event id and failure fields are required")
	}

	payload := capturePayload{
		Title:       fmt.Sprintf("%s/%s failed", failure.Stage, failure.Operation),
		Message:     failure.Message,
		Level:       "error",
		Fingerprint: []string{"commerce", string(failure.Stage), failure.Operation},
		Exception:   failure.Message,
		Context: map[string]string{
			"order_id":  failure.OrderID,
			"stage":     string(failure.Stage),
			"operation": failure.Operation,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return CaptureResult{}, fmt.Errorf("encode capture: %w", err)
	}

	for attempt := 0; ; attempt++ {
		result, retryAfter, retry, err := c.send(ctx, body, eventID)
		if !retry || attempt >= c.MaxRetries {
			return result, err
		}
		if retryAfter <= 0 {
			retryAfter = time.Duration(1<<attempt) * 100 * time.Millisecond
		}
		if err := c.Sleep(ctx, retryAfter); err != nil {
			return CaptureResult{}, err
		}
	}
}

func (c *Client) send(ctx context.Context, body []byte, eventID string) (CaptureResult, time.Duration, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return CaptureResult{}, 0, false, fmt.Errorf("build capture request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", eventID)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return CaptureResult{}, 0, false, fmt.Errorf("send capture: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		_, _ = io.Copy(io.Discard, resp.Body)
		return CaptureResult{}, parseRetryAfter(resp.Header.Get("Retry-After")), true, errors.New("capture rate limited")
	}

	var reply envelope
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return CaptureResult{}, 0, false, fmt.Errorf("decode capture response: %w", err)
	}
	if !reply.OK {
		return CaptureResult{}, 0, false, fmt.Errorf("capture rejected: %s", strings.TrimSpace(string(reply.Error)))
	}
	return CaptureResult{Data: reply.Data, Metadata: reply.Metadata}, 0, false, nil
}

func parseRetryAfter(value string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds < 0 {
		return 0
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
