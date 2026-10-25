package task

import "fmt"

// localKeyPrefix is the project key of local tasks.
//
// TODO: Decide in the task API issue whether this moves to the TOML config.
const localKeyPrefix = "WHF"

// NewID returns the task ID "{originated}-{key}", which keeps keys from
// different origins apart.
func NewID(originated Origin, key string) string {
	return string(originated) + "-" + key
}

// LocalKey returns the key of the nth local task, such as "WHF-001". Numbers
// are zero-padded to three digits and grow past them, as in "WHF-1000".
func LocalKey(n int64) string {
	return fmt.Sprintf("%s-%03d", localKeyPrefix, n)
}
