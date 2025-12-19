package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

type RemindTimeClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewRemindTimeClient(baseURL string) *RemindTimeClient {
	return &RemindTimeClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *RemindTimeClient) GetRemindsByTimeRange(ctx context.Context, start, end time.Time) (*RemindsResponse, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %w", err)
	}

	u.Path = "/api/v1/reminds"
	q := u.Query()
	q.Set("start", start.Format(time.RFC3339))
	q.Set("end", end.Format(time.RFC3339))
	u.RawQuery = q.Encode()

	slog.Debug("fetching reminds from RemindTimeManagement",
		slog.String("url", u.String()),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Error("failed to send request to RemindTimeManagement",
			slog.String("url", u.String()),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("unexpected status code from RemindTimeManagement",
			slog.String("url", u.String()),
			slog.Int("status_code", resp.StatusCode),
		)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var remindsResp RemindsResponse
	if err := json.NewDecoder(resp.Body).Decode(&remindsResp); err != nil {
		slog.Error("failed to decode response from RemindTimeManagement",
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	slog.Debug("successfully fetched reminds",
		slog.Int("count", remindsResp.Count),
	)

	return &remindsResp, nil
}

func (c *RemindTimeClient) UpdateThrottled(ctx context.Context, id string, throttled bool) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("failed to parse base URL: %w", err)
	}

	u.Path = fmt.Sprintf("/api/v1/reminds/%s/throttled", id)

	slog.Debug("updating throttled flag",
		slog.String("remind_id", id),
		slog.Bool("throttled", throttled),
		slog.String("url", u.String()),
	)

	reqBody := UpdateThrottledRequest{
		Throttled: throttled,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Error("failed to send update throttled request",
			slog.String("remind_id", id),
			slog.String("url", u.String()),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		slog.Error("unexpected status code when updating throttled flag",
			slog.String("remind_id", id),
			slog.String("url", u.String()),
			slog.Int("status_code", resp.StatusCode),
		)
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	slog.Debug("successfully updated throttled flag",
		slog.String("remind_id", id),
	)

	return nil
}
