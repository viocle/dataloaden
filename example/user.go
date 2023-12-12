//go:generate go run github.com/viocle/dataloaden UserLoader string *github.com/viocle/dataloaden/example.User
//go:generate go run github.com/viocle/dataloaden UserByIDAndOrgLoader UserByIDAndOrg *github.com/viocle/dataloaden/example.User true
//go:generate go run github.com/viocle/dataloaden UserValueByIDAndOrgLoader UserByIDAndOrg github.com/viocle/dataloaden/example.User true
//go:generate go run github.com/viocle/dataloaden UserIntLoader int *github.com/viocle/dataloaden/example.User
//go:generate go run github.com/viocle/dataloaden UserFloatLoader float64 *github.com/viocle/dataloaden/example.User

package example

import (
	"time"
)

// User is some kind of database backed model
type User struct {
	ID    string
	OrgID string
	Name  string
}

type UserByIDAndOrg struct {
	ID    string `json:"id"`
	OrgID string `json:"oid"`
}

// NewUserLoaderExample will collect user requests for 2 milliseconds and send them as a single batch to the fetch func
// normally fetch would be a database call.
func NewUserLoaderExample() *UserLoader {
	return NewUserLoader(UserLoaderConfig{
		Wait:     2 * time.Millisecond,
		MaxBatch: 100,
		Fetch: func(keys []string) ([]*User, []error) {
			users := make([]*User, len(keys))
			errors := make([]error, len(keys))

			for i, key := range keys {
				users[i] = &User{ID: key, Name: "user " + key}
			}
			return users, errors
		},
	})
}
