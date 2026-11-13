package task

type Status string

const (
	StatusTodo       = "todo"
	StatusInProgress = "in-progress"
	StatusDone       = "done"
	StatusCanceled   = "canceled"
)

// Closed reports whether s is done or canceled.
func (s Status) Closed() bool {
	return s == StatusDone || s == StatusCanceled
}
