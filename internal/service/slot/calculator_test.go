package slot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

// mockCounter is a simple mock implementation of Counter for testing.
type mockCounter struct {
	counts map[string]int
	err    error
}

func newMockCounter() *mockCounter {
	return &mockCounter{
		counts: make(map[string]int),
	}
}

func (m *mockCounter) GetTotalCountForMinute(ctx context.Context, minuteKey string) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.counts[minuteKey], nil
}

func (m *mockCounter) setCount(minuteKey string, count int) {
	m.counts[minuteKey] = count
}

func (m *mockCounter) setError(err error) {
	m.err = err
}

func TestCalculator_FindSlot_NoCapConfigured(t *testing.T) {
	mock := newMockCounter()
	calc := NewCalculator(mock, 0, nil) // cap = 0 means no cap

	originalTime := time.Now().Truncate(time.Minute)
	got, shifted := calc.FindSlot(context.Background(), originalTime, 300, domain.LaneLoose)

	if !got.Equal(originalTime) {
		t.Errorf("FindSlot() time = %v, want %v", got, originalTime)
	}
	if shifted {
		t.Error("FindSlot() shifted = true, want false")
	}
}

func TestCalculator_FindSlot_StrictLane_UnderCap(t *testing.T) {
	mock := newMockCounter()
	calc := NewCalculator(mock, 10, nil) // cap = 10

	originalTime := time.Now().Truncate(time.Minute)
	minuteKey := domain.MinuteKey(originalTime)
	mock.setCount(minuteKey, 5) // Under cap

	// Strict lane with 2 min window (120 seconds)
	got, shifted := calc.FindSlot(context.Background(), originalTime, 120, domain.LaneStrict)

	if !got.Equal(originalTime) {
		t.Errorf("FindSlot() time = %v, want %v", got, originalTime)
	}
	if shifted {
		t.Error("FindSlot() shifted = true, want false")
	}
}

func TestCalculator_FindSlot_StrictLane_CapExceeded_ShiftsWithinWindow(t *testing.T) {
	mock := newMockCounter()
	calc := NewCalculator(mock, 10, nil) // cap = 10

	originalTime := time.Now().Truncate(time.Minute)
	originalKey := domain.MinuteKey(originalTime)
	plusOneKey := domain.MinuteKey(originalTime.Add(time.Minute))
	plusTwoKey := domain.MinuteKey(originalTime.Add(2 * time.Minute))

	// Original minute is full, +1 minute has space
	mock.setCount(originalKey, 10)
	mock.setCount(plusOneKey, 5)
	mock.setCount(plusTwoKey, 10)

	// Strict lane with 2 min window (120 seconds)
	got, shifted := calc.FindSlot(context.Background(), originalTime, 120, domain.LaneStrict)

	expectedTime := originalTime.Add(time.Minute)
	if !got.Equal(expectedTime) {
		t.Errorf("FindSlot() time = %v, want %v", got, expectedTime)
	}
	if !shifted {
		t.Error("FindSlot() shifted = false, want true")
	}
}

func TestCalculator_FindSlot_StrictLane_CapExceeded_NoSlotAvailable(t *testing.T) {
	mock := newMockCounter()
	calc := NewCalculator(mock, 10, nil) // cap = 10

	originalTime := time.Now().Truncate(time.Minute)
	originalKey := domain.MinuteKey(originalTime)
	plusOneKey := domain.MinuteKey(originalTime.Add(time.Minute))
	plusTwoKey := domain.MinuteKey(originalTime.Add(2 * time.Minute))

	// All minutes within slide window are full
	mock.setCount(originalKey, 10)
	mock.setCount(plusOneKey, 10)
	mock.setCount(plusTwoKey, 10)

	// Strict lane with 2 min window (120 seconds)
	got, shifted := calc.FindSlot(context.Background(), originalTime, 120, domain.LaneStrict)

	// Should return original time when no slot available
	if !got.Equal(originalTime) {
		t.Errorf("FindSlot() time = %v, want %v", got, originalTime)
	}
	if shifted {
		t.Error("FindSlot() shifted = true, want false (no slot available)")
	}
}

func TestCalculator_FindSlot_StrictLane_NoSlideWindow(t *testing.T) {
	mock := newMockCounter()
	calc := NewCalculator(mock, 10, nil)

	originalTime := time.Now().Truncate(time.Minute)
	minuteKey := domain.MinuteKey(originalTime)
	mock.setCount(minuteKey, 10) // At cap

	// Slide window = 0 means no shifting allowed
	got, shifted := calc.FindSlot(context.Background(), originalTime, 0, domain.LaneStrict)

	if !got.Equal(originalTime) {
		t.Errorf("FindSlot() time = %v, want %v", got, originalTime)
	}
	if shifted {
		t.Error("FindSlot() shifted = true, want false (no slide window)")
	}
}

func TestCalculator_FindSlot_StrictLane_Error_ReturnsOriginal(t *testing.T) {
	mock := newMockCounter()
	mock.setError(errors.New("connection error"))
	calc := NewCalculator(mock, 10, nil)

	originalTime := time.Now().Truncate(time.Minute)

	got, shifted := calc.FindSlot(context.Background(), originalTime, 120, domain.LaneStrict)

	if !got.Equal(originalTime) {
		t.Errorf("FindSlot() time = %v, want %v", got, originalTime)
	}
	if shifted {
		t.Error("FindSlot() shifted = true, want false on error")
	}
}

func TestCalculator_FindSlot_LooseLane_UnderCap(t *testing.T) {
	mock := newMockCounter()
	calc := NewCalculator(mock, 10, nil)

	originalTime := time.Now().Truncate(time.Minute)
	minuteKey := domain.MinuteKey(originalTime)
	mock.setCount(minuteKey, 5)

	got, shifted := calc.FindSlot(context.Background(), originalTime, 300, domain.LaneLoose)

	if !got.Equal(originalTime) {
		t.Errorf("FindSlot() time = %v, want %v", got, originalTime)
	}
	if shifted {
		t.Error("FindSlot() shifted = true, want false")
	}
}

func TestCalculator_FindSlot_LooseLane_CapExceeded_ShiftsWithinWindow(t *testing.T) {
	mock := newMockCounter()
	calc := NewCalculator(mock, 10, nil)

	originalTime := time.Now().Truncate(time.Minute)
	originalKey := domain.MinuteKey(originalTime)
	plusOneKey := domain.MinuteKey(originalTime.Add(time.Minute))
	plusTwoKey := domain.MinuteKey(originalTime.Add(2 * time.Minute))
	plusThreeKey := domain.MinuteKey(originalTime.Add(3 * time.Minute))

	// Original and +1 are full, +2 has space
	mock.setCount(originalKey, 10)
	mock.setCount(plusOneKey, 10)
	mock.setCount(plusTwoKey, 5)
	mock.setCount(plusThreeKey, 10)

	// Slide window of 300 seconds = 5 minutes
	got, shifted := calc.FindSlot(context.Background(), originalTime, 300, domain.LaneLoose)

	expectedTime := originalTime.Add(2 * time.Minute)
	if !got.Equal(expectedTime) {
		t.Errorf("FindSlot() time = %v, want %v", got, expectedTime)
	}
	if !shifted {
		t.Error("FindSlot() shifted = false, want true")
	}
}

func TestCalculator_FindSlot_LooseLane_CapExceeded_NoSlotInWindow(t *testing.T) {
	mock := newMockCounter()
	calc := NewCalculator(mock, 10, nil)

	originalTime := time.Now().Truncate(time.Minute)

	// All minutes within window are full
	for i := 0; i <= 5; i++ {
		key := domain.MinuteKey(originalTime.Add(time.Duration(i) * time.Minute))
		mock.setCount(key, 10)
	}

	// Slide window of 180 seconds = 3 minutes
	got, shifted := calc.FindSlot(context.Background(), originalTime, 180, domain.LaneLoose)

	// Should return original time when no slot available within window
	if !got.Equal(originalTime) {
		t.Errorf("FindSlot() time = %v, want %v", got, originalTime)
	}
	if shifted {
		t.Error("FindSlot() shifted = true, want false (no slot in window)")
	}
}

func TestCalculator_FindSlot_LooseLane_NoSlideWindow(t *testing.T) {
	mock := newMockCounter()
	calc := NewCalculator(mock, 10, nil)

	originalTime := time.Now().Truncate(time.Minute)
	minuteKey := domain.MinuteKey(originalTime)
	mock.setCount(minuteKey, 10) // At cap

	// Slide window = 0 means no shifting allowed
	got, shifted := calc.FindSlot(context.Background(), originalTime, 0, domain.LaneLoose)

	if !got.Equal(originalTime) {
		t.Errorf("FindSlot() time = %v, want %v", got, originalTime)
	}
	if shifted {
		t.Error("FindSlot() shifted = true, want false (no slide window)")
	}
}

func TestCalculator_FindSlot_LooseLane_Error_ReturnsOriginal(t *testing.T) {
	mock := newMockCounter()
	mock.setError(errors.New("connection error"))
	calc := NewCalculator(mock, 10, nil)

	originalTime := time.Now().Truncate(time.Minute)

	got, shifted := calc.FindSlot(context.Background(), originalTime, 300, domain.LaneLoose)

	if !got.Equal(originalTime) {
		t.Errorf("FindSlot() time = %v, want %v", got, originalTime)
	}
	if shifted {
		t.Error("FindSlot() shifted = true, want false on error")
	}
}

// TestCalculator_UnifiedBehavior verifies that both lanes use slideWindowSeconds
func TestCalculator_UnifiedBehavior(t *testing.T) {
	mock := newMockCounter()
	calc := NewCalculator(mock, 10, nil)

	originalTime := time.Now().Truncate(time.Minute)
	originalKey := domain.MinuteKey(originalTime)
	plusOneKey := domain.MinuteKey(originalTime.Add(time.Minute))
	plusTwoKey := domain.MinuteKey(originalTime.Add(2 * time.Minute))
	plusThreeKey := domain.MinuteKey(originalTime.Add(3 * time.Minute))

	// Original is full, +3 has space (outside strict's old 2-min limit)
	mock.setCount(originalKey, 10)
	mock.setCount(plusOneKey, 10)
	mock.setCount(plusTwoKey, 10)
	mock.setCount(plusThreeKey, 5)

	// Even strict lane should shift to +3 if slideWindowSeconds allows it
	got, shifted := calc.FindSlot(context.Background(), originalTime, 300, domain.LaneStrict)

	expectedTime := originalTime.Add(3 * time.Minute)
	if !got.Equal(expectedTime) {
		t.Errorf("FindSlot() time = %v, want %v (strict should shift beyond old 2-min limit)", got, expectedTime)
	}
	if !shifted {
		t.Error("FindSlot() shifted = false, want true")
	}
}
