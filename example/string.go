//go:generate go run github.com/viocle/dataloaden StringLoader string string

package example

import (
	"time"
)

// NewStringLoaderExample will collect user requests for 2 milliseconds and send them as a single batch to the fetch func
// normally fetch would be a database call.
func NewStringLoaderExample() *StringLoader {
	return NewStringLoader(StringLoaderConfig{
		Wait:     2 * time.Millisecond,
		MaxBatch: 100,
		Fetch: func(keys []string) ([]string, []error) {
			users := make([]string, len(keys))
			errors := make([]error, len(keys))

			for i, key := range keys {
				users[i] = "user " + key
			}
			return users, errors
		},
	})
}
