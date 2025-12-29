package domain

// Lane represents the classification of a notification for throttling purposes.
type Lane string

const (
	LaneStrict Lane = "strict"
	LaneLoose  Lane = "loose"
)

func (l Lane) String() string {
	return string(l)
}

func (l Lane) IsStrict() bool {
	return l == LaneStrict
}

func (l Lane) IsLoose() bool {
	return l == LaneLoose
}
