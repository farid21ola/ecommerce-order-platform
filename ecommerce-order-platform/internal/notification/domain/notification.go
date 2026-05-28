package domain

const (
	TaskStatusSent   = "SENT"
	TaskStatusFailed = "FAILED"
)

type Task struct {
	ID      string
	OrderID string
	EventID string
	Status  string
}
