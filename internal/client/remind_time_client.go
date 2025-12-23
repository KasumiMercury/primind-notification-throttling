package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	commonv1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/common/v1"
	remindv1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/remind/v1"
	pjson "github.com/KasumiMercury/primind-notification-throttling/internal/proto"
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("failed to read response body from RemindTimeManagement",
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var protoResp remindv1.RemindsResponse
	if err := pjson.Unmarshal(body, &protoResp); err != nil {
		slog.Error("failed to decode response from RemindTimeManagement",
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert proto response to client response type
	remindsResp := protoRemindsToClient(&protoResp)

	slog.Debug("successfully fetched reminds",
		slog.Int("count", remindsResp.Count),
	)

	return remindsResp, nil
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

	protoReq := &remindv1.UpdateThrottledRequest{
		Throttled: throttled,
	}

	body, err := pjson.Marshal(protoReq)
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

func protoRemindsToClient(protoResp *remindv1.RemindsResponse) *RemindsResponse {
	reminds := make([]RemindResponse, 0, len(protoResp.Reminds))
	for _, r := range protoResp.Reminds {
		devices := make([]DeviceResponse, 0, len(r.Devices))
		for _, d := range r.Devices {
			devices = append(devices, DeviceResponse{
				DeviceID: d.DeviceId,
				FCMToken: d.FcmToken,
			})
		}

		var remindTime time.Time
		if r.Time != nil {
			remindTime = r.Time.AsTime()
		}
		var createdAt time.Time
		if r.CreatedAt != nil {
			createdAt = r.CreatedAt.AsTime()
		}
		var updatedAt time.Time
		if r.UpdatedAt != nil {
			updatedAt = r.UpdatedAt.AsTime()
		}

		reminds = append(reminds, RemindResponse{
			ID:        r.Id,
			Time:      remindTime,
			UserID:    r.UserId,
			Devices:   devices,
			TaskID:    r.TaskId,
			TaskType:  taskTypeToString(r.TaskType),
			Throttled: r.Throttled,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}

	return &RemindsResponse{
		Reminds: reminds,
		Count:   int(protoResp.Count),
	}
}

func taskTypeToString(t commonv1.TaskType) string {
	name := t.String()
	cutedName, _ := strings.CutPrefix(name, "TASK_TYPE_")
	return strings.ToLower(cutedName)
}
