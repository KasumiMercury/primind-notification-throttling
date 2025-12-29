package domain

import "errors"

var (
	ErrPacketNotFound        = errors.New("packet not found")
	ErrPlanNotFound          = errors.New("plan not found")
	ErrPlannedPacketNotFound = errors.New("planned packet not found")
)
