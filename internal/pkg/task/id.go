package task

import "fmt"

// NewID returns the ID of the task identified by key in origin, in the
// "{origin}-{key}" form shared by every origin.
func NewID(origin Origin, key string) string {
	return string(origin) + "-" + key
}

// LocalKey returns the key of the n-th local task, "WHF-" followed by n
// zero-padded to three digits; numbers past 999 get more digits.
func LocalKey(n int64) string {
	return fmt.Sprintf("WHF-%03d", n)
}
