package stub

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/common/v1"
	remindv1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/remind/v1"
	pjson "github.com/KasumiMercury/primind-notification-throttling/internal/proto"
)

type Handler struct {
	storage *BucketStorage
}

func NewHandler(storage *BucketStorage) *Handler {
	return &Handler{storage: storage}
}

func (h *Handler) HandleReset(c *gin.Context) {
	runID := c.DefaultQuery("run_id", "default")

	h.storage.Reset(runID)

	slog.Info("reset data", slog.String("run_id", runID))

	c.JSON(http.StatusOK, gin.H{
		"status": "reset complete",
		"run_id": runID,
	})
}

func (h *Handler) HandleSeed(c *gin.Context) {
	runID := c.DefaultQuery("run_id", "default")

	var req SeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	totalCount := 0
	for _, sb := range req.Buckets {
		startTime, err := time.Parse(time.RFC3339, sb.StartTime)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_time: " + sb.StartTime})
			return
		}
		endTime, err := time.Parse(time.RFC3339, sb.EndTime)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end_time: " + sb.EndTime})
			return
		}

		taskType := sb.TaskType
		if taskType == "" {
			taskType = "reminder"
		}

		h.storage.AddBucket(runID, &Bucket{
			StartTime:        startTime,
			EndTime:          endTime,
			Count:            sb.Count,
			SlideWindowWidth: sb.SlideWindowWidth,
			TaskType:         taskType,
			Devices:          sb.Devices,
		})

		totalCount += sb.Count
	}

	slog.Info("seeded data",
		slog.String("run_id", runID),
		slog.Int("bucket_count", len(req.Buckets)),
		slog.Int("total_remind_count", totalCount),
	)

	c.JSON(http.StatusOK, gin.H{
		"status":       "seeded",
		"run_id":       runID,
		"bucket_count": len(req.Buckets),
		"total_count":  totalCount,
	})
}

// GET /api/v1/reminds?start=...&end=...&run_id=...
// Returns proto-compatible JSON
func (h *Handler) HandleGetReminds(c *gin.Context) {
	runID := c.DefaultQuery("run_id", "default")
	startStr := c.Query("start")
	endStr := c.Query("end")

	if startStr == "" || endStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start and end query parameters are required"})
		return
	}

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time format"})
		return
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time format"})
		return
	}

	reminds := h.storage.GetRemindsInRange(runID, start, end)

	slog.Debug("get reminds",
		slog.String("run_id", runID),
		slog.Time("start", start),
		slog.Time("end", end),
		slog.Int("count", len(reminds)),
	)

	protoReminds := make([]*remindv1.Remind, 0, len(reminds))
	for _, r := range reminds {
		devices := make([]*remindv1.Device, 0, len(r.Devices))
		for _, d := range r.Devices {
			devices = append(devices, &remindv1.Device{
				DeviceId: d.DeviceID,
				FcmToken: d.FCMToken,
			})
		}

		protoReminds = append(protoReminds, &remindv1.Remind{
			Id:               r.ID,
			Time:             timestamppb.New(r.Time),
			UserId:           r.UserID,
			Devices:          devices,
			TaskId:           r.TaskID,
			TaskType:         stringToTaskType(r.TaskType),
			Throttled:        r.Throttled,
			SlideWindowWidth: r.SlideWindowWidth,
			CreatedAt:        timestamppb.New(r.CreatedAt),
			UpdatedAt:        timestamppb.New(r.UpdatedAt),
		})
	}

	resp := &remindv1.RemindsResponse{
		Reminds: protoReminds,
		Count:   int32(len(reminds)),
	}

	respBytes, err := pjson.Marshal(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal response"})
		return
	}

	c.Data(http.StatusOK, "application/json", respBytes)
}

// POST /api/v1/reminds/:id/throttled?run_id=...
func (h *Handler) HandleUpdateThrottled(c *gin.Context) {
	id := c.Param("id")
	runID := c.DefaultQuery("run_id", "default")

	var req UpdateThrottledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.storage.SetThrottled(runID, id, req.Throttled)

	slog.Debug("update throttled",
		slog.String("run_id", runID),
		slog.String("remind_id", id),
		slog.Bool("throttled", req.Throttled),
	)

	c.Status(http.StatusNoContent)
}

// stringToTaskType converts a string to a proto TaskType enum.
func stringToTaskType(s string) commonv1.TaskType {
	upper := "TASK_TYPE_" + strings.ToUpper(s)
	if v, ok := commonv1.TaskType_value[upper]; ok {
		return commonv1.TaskType(v)
	}
	return commonv1.TaskType_TASK_TYPE_UNSPECIFIED
}
