//go:generate go run github.com/viocle/dataloaden UserSliceLoader int []github.com/viocle/dataloaden/example.User
//go:generate go run github.com/viocle/dataloaden UserIntSliceLoader int []*github.com/viocle/dataloaden/example.User

package slice

import (
	"strconv"
	"time"

	"github.com/viocle/dataloaden/example"
)

// NewLoader returns a new *UserSliceLoader
func NewLoader() *UserSliceLoader {
	return NewUserSliceLoader(UserSliceLoaderConfig{
		Wait:     2 * time.Millisecond,
		MaxBatch: 100,
		Fetch: func(keys []int) ([][]example.User, []error) {
			users := make([][]example.User, len(keys))
			errors := make([]error, len(keys))

			for i, key := range keys {
				users[i] = []example.User{{ID: strconv.Itoa(key), Name: "user " + strconv.Itoa(key)}}
			}
			return users, errors
		},
	})
}
