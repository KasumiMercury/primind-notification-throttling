package stub

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type Bucket struct {
	StartTime        time.Time
	EndTime          time.Time
	Count            int
	SlideWindowWidth int32
	TaskType         string
	Devices          []DeviceResponse
}

type BucketStorage struct {
	mu           sync.RWMutex
	buckets      map[string][]*Bucket       // runID -> buckets
	throttledIDs map[string]map[string]bool // runID -> remindID -> throttled
}

func NewBucketStorage() *BucketStorage {
	return &BucketStorage{
		buckets:      make(map[string][]*Bucket),
		throttledIDs: make(map[string]map[string]bool),
	}
}

func (s *BucketStorage) Reset(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.buckets, runID)
	delete(s.throttledIDs, runID)
}

func (s *BucketStorage) ResetAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buckets = make(map[string][]*Bucket)
	s.throttledIDs = make(map[string]map[string]bool)
}

func (s *BucketStorage) AddBucket(runID string, bucket *Bucket) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buckets[runID] = append(s.buckets[runID], bucket)
}

func (s *BucketStorage) GetRemindsInRange(runID string, start, end time.Time) []RemindResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	buckets := s.buckets[runID]
	if len(buckets) == 0 {
		return []RemindResponse{}
	}

	var reminds []RemindResponse

	for _, bucket := range buckets {
		if bucket.EndTime.Before(start) || bucket.StartTime.After(end) {
			continue
		}

		reminds = append(reminds, s.generateRemindsForBucket(runID, bucket, start, end)...)
	}

	return reminds
}

func (s *BucketStorage) generateRemindsForBucket(runID string, bucket *Bucket, start, end time.Time) []RemindResponse {
	if bucket.Count == 0 {
		return nil
	}

	reminds := make([]RemindResponse, 0)

	bucketDuration := bucket.EndTime.Sub(bucket.StartTime)
	if bucketDuration <= 0 {
		bucketDuration = time.Minute
	}

	interval := bucketDuration / time.Duration(bucket.Count)
	if interval == 0 {
		interval = time.Second
	}

	now := time.Now()

	for i := 0; i < bucket.Count; i++ {
		remindTime := bucket.StartTime.Add(time.Duration(i) * interval)

		if remindTime.Before(start) || !remindTime.Before(end) {
			continue
		}

		id := generateRemindID(runID, bucket.StartTime, bucket.SlideWindowWidth, i)

		throttled := false
		if throttledMap, exists := s.throttledIDs[runID]; exists {
			throttled = throttledMap[id]
		}

		devices := bucket.Devices
		if len(devices) == 0 {
			devices = []DeviceResponse{
				{DeviceID: "device-1", FCMToken: "fcm-token-" + id},
			}
		}

		reminds = append(reminds, RemindResponse{
			ID:               id,
			Time:             remindTime,
			UserID:           "user-" + id[:8],
			Devices:          devices,
			TaskID:           "task-" + id[:8],
			TaskType:         bucket.TaskType,
			Throttled:        throttled,
			SlideWindowWidth: bucket.SlideWindowWidth,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}

	return reminds
}

func (s *BucketStorage) SetThrottled(runID, remindID string, throttled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.throttledIDs[runID] == nil {
		s.throttledIDs[runID] = make(map[string]bool)
	}
	s.throttledIDs[runID][remindID] = throttled
}

func generateRemindID(runID string, bucketStart time.Time, slideWindowWidth int32, index int) string {
	input := fmt.Sprintf("%s-%s-%d-%d", runID, bucketStart.Format("20060102150405"), slideWindowWidth, index)
	hash := sha256.Sum256([]byte(input))
	hashStr := hex.EncodeToString(hash[:8])
	return fmt.Sprintf("%s-%s-%s", runID, bucketStart.Format("20060102150405"), hashStr)
}
