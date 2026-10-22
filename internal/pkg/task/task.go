package task

import "time"

type Task struct {
	ID          string
	Originated  Origin
	Title       string
	Description string
	Status      Status
	StartDate   *time.Time
	DueDate     *time.Time
}
