package domain

import "errors"

var (
	ErrPacketAlreadyCommitted = errors.New("packet already committed")
	ErrCapExceeded            = errors.New("per-minute cap exceeded")
	ErrPacketNotFound         = errors.New("packet not found")
	ErrPlanNotFound           = errors.New("plan not found")
	ErrPlannedPacketNotFound  = errors.New("planned packet not found")
)
