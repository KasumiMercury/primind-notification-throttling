package sliding

import (
	"container/heap"
	"time"
)

type PriorityQueue struct {
	items    []*PriorityItem
	slotTime time.Time
}

func NewPriorityQueue(slotTime time.Time) *PriorityQueue {
	return &PriorityQueue{
		items:    make([]*PriorityItem, 0),
		slotTime: slotTime,
	}
}

func (pq *PriorityQueue) Len() int {
	return len(pq.items)
}

func (pq *PriorityQueue) Less(i, j int) bool {
	a, b := pq.items[i], pq.items[j]

	// Earliest deadline first (EDF)
	if !a.Latest.Equal(b.Latest) {
		return a.Latest.Before(b.Latest)
	}

	// Lower move cost wins
	return pq.MoveCost(a) < pq.MoveCost(b)
}

func (pq *PriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
	pq.items[i].Index = i
	pq.items[j].Index = j
}

func (pq *PriorityQueue) Push(x any) {
	item := x.(*PriorityItem)
	item.Index = len(pq.items)
	pq.items = append(pq.items, item)
}

func (pq *PriorityQueue) Pop() any {
	old := pq.items
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	pq.items = old[0 : n-1]
	return item
}

func (pq *PriorityQueue) MoveCost(item *PriorityItem) int {
	slotMinute := pq.slotTime.Truncate(time.Minute).Unix() / 60
	releaseMinute := item.Release.Truncate(time.Minute).Unix() / 60

	if slotMinute >= releaseMinute {
		// Late shift: heavy penalty (x10)
		return int(slotMinute-releaseMinute) * 10
	}
	// Early shift: light penalty (x1)
	return int(releaseMinute - slotMinute)
}

func (pq *PriorityQueue) SetSlotTime(t time.Time) {
	pq.slotTime = t
	heap.Init(pq)
}

func (pq *PriorityQueue) SlotTime() time.Time {
	return pq.slotTime
}
