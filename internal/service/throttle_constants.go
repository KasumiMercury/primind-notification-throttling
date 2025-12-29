package service

const (
	// StrictSlideWindowThreshold is the maximum slide window width (in seconds)
	// for a task to be classified as strict lane.
	// This corresponds to a total window of 5 minutes (one-sided <= 2.5 minutes = 150 seconds).
	// Tasks with slide window <= 150 seconds are considered time-critical.
	StrictSlideWindowThreshold = 150
)
