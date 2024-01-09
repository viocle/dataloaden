package slice

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/viocle/dataloaden/example"
)

// BenchmarkLoader benchmark the standard loader
func BenchmarkSliceLoader(b *testing.B) {
	var fetches [][]int
	var mu sync.Mutex

	dl := NewUserSliceLoader(UserSliceLoaderConfig{
		Wait:     500 * time.Nanosecond,
		MaxBatch: 100,
		Fetch: func(keys []int) (users [][]example.User, errors []error) {
			mu.Lock()
			fetches = append(fetches, keys)
			mu.Unlock()

			users = make([][]example.User, len(keys))
			errors = make([]error, len(keys))

			for i, key := range keys {
				if key%10 == 0 { // anything ending in zero is bad
					errors[i] = fmt.Errorf("users not found")
				} else {
					users[i] = []example.User{
						{ID: strconv.Itoa(key), Name: "user " + strconv.Itoa(key)},
						{ID: strconv.Itoa(key), Name: "user " + strconv.Itoa(key)},
					}
				}
			}
			return users, errors
		},
	})

	b.Run("caches", func(b *testing.B) {
		thunks := make([]func() ([]example.User, error), b.N)
		for i := 0; i < b.N; i++ {
			_, thunks[i] = dl.LoadThunk(rand.Intn(5000))
		}

		for i := 0; i < b.N; i++ {
			if thunks[i] != nil {
				thunks[i]()
			}
		}
	})

	b.Run("random spread", func(b *testing.B) {
		thunks := make([]func() ([]example.User, error), b.N)
		for i := 0; i < b.N; i++ {
			_, thunks[i] = dl.LoadThunk(rand.Intn(5000))
		}

		for i := 0; i < b.N; i++ {
			if thunks[i] != nil {
				thunks[i]()
			}
		}
	})

	b.Run("concurrently", func(b *testing.B) {
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				for j := 0; j < b.N; j++ {
					dl.Load(rand.Intn(5000))
				}
				wg.Done()
			}()
		}
		wg.Wait()
	})
}
