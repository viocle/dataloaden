package example

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"
)

// BenchmarkLoader benchmark the standard loader
func BenchmarkLoader(b *testing.B) {
	dl := NewUserLoader(UserLoaderConfig{
		Wait:     500 * time.Nanosecond,
		MaxBatch: 100,
		Fetch: func(keys []string) ([]*User, []error) {
			users := make([]*User, len(keys))
			errors := make([]error, len(keys))

			for i, key := range keys {
				if rand.Int()%100 == 1 {
					errors[i] = fmt.Errorf("user not found")
				} else if rand.Int()%100 == 1 {
					users[i] = nil
				} else {
					users[i] = &User{ID: key, Name: "user " + key}
				}
			}
			return users, errors
		},
	})

	b.Run("caches", func(b *testing.B) {
		thunks := make([]func() (*User, error), b.N)
		for i := 0; i < b.N; i++ {
			_, thunks[i] = dl.LoadThunk(strconv.Itoa(rand.Int() % 300))
		}

		for i := 0; i < b.N; i++ {
			if thunks[i] != nil {
				thunks[i]()
			}
		}
	})

	b.Run("random spread", func(b *testing.B) {
		thunks := make([]func() (*User, error), b.N)
		for i := 0; i < b.N; i++ {
			_, thunks[i] = dl.LoadThunk(strconv.Itoa(rand.Int()))
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
					dl.Load(strconv.Itoa(rand.Int()))
				}
				wg.Done()
			}()
		}
		wg.Wait()
	})
}

// BenchmarkLoader benchmark the standard loader
func BenchmarkLoaderStruct(b *testing.B) {
	orgIDs := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}
	dl := NewUserByIDAndOrgLoader(UserByIDAndOrgLoaderConfig{
		Wait:     500 * time.Nanosecond,
		MaxBatch: 100,
		Fetch: func(keys []UserByIDAndOrg) ([]*User, []error) {
			users := make([]*User, len(keys))
			errors := make([]error, len(keys))

			for i, key := range keys {
				if rand.Int()%100 == 1 {
					errors[i] = fmt.Errorf("user not found")
				} else if rand.Int()%100 == 1 {
					users[i] = nil
				} else {
					users[i] = &User{ID: key.ID, OrgID: key.OrgID, Name: "user " + key.ID}
				}
			}
			return users, errors
		},
	})

	b.Run("caches", func(b *testing.B) {
		thunks := make([]func() (*User, error), b.N)
		for i := 0; i < b.N; i++ {
			_, f := dl.LoadThunk(UserByIDAndOrg{ID: strconv.Itoa(rand.Int() % 300), OrgID: orgIDs[rand.Intn(len(orgIDs)-1)]})
			if f != nil {
				thunks[i] = f
			}
		}

		for i := 0; i < b.N; i++ {
			if thunks[i] != nil {
				thunks[i]()
			}
		}
	})

	b.Run("random spread", func(b *testing.B) {
		thunks := make([]func() (*User, error), b.N)
		for i := 0; i < b.N; i++ {
			_, f := dl.LoadThunk(UserByIDAndOrg{ID: strconv.Itoa(rand.Int() % 300), OrgID: orgIDs[rand.Intn(len(orgIDs)-1)]})
			if f != nil {
				thunks[i] = f
			}
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
					dl.Load(UserByIDAndOrg{ID: strconv.Itoa(rand.Int() % 300), OrgID: orgIDs[rand.Intn(len(orgIDs)-1)]})
				}
				wg.Done()
			}()
		}
		wg.Wait()
	})
}

// BenchmarkLoaderExpires benchmark the loader when using the cache expiration, even though this isn't exactly the intended use case
func BenchmarkLoaderExpires(b *testing.B) {
	dl := NewUserLoader(UserLoaderConfig{
		Wait:        500 * time.Nanosecond,
		MaxBatch:    100,
		ExpireAfter: time.Minute,
		Fetch: func(keys []string) ([]*User, []error) {
			users := make([]*User, len(keys))
			errors := make([]error, len(keys))

			for i, key := range keys {
				if rand.Int()%100 == 1 {
					errors[i] = fmt.Errorf("user not found")
				} else if rand.Int()%100 == 1 {
					users[i] = nil
				} else {
					users[i] = &User{ID: key, Name: "user " + key}
				}
			}
			return users, errors
		},
	})

	b.Run("caches", func(b *testing.B) {
		thunks := make([]func() (*User, error), b.N)
		for i := 0; i < b.N; i++ {
			// thunks[i] = dl.LoadThunk(strconv.Itoa(rand.Int() % 300))
			_, f := dl.LoadThunk(strconv.Itoa(rand.Int() % 300))
			if f != nil {
				thunks[i] = f
			}
		}

		for i := 0; i < b.N; i++ {
			if thunks[i] != nil {
				thunks[i]()
			}
		}
	})

	b.Run("random spread", func(b *testing.B) {
		thunks := make([]func() (*User, error), b.N)
		for i := 0; i < b.N; i++ {
			// thunks[i] = dl.LoadThunk(strconv.Itoa(rand.Int()))
			_, f := dl.LoadThunk(strconv.Itoa(rand.Int()))
			if f != nil {
				thunks[i] = f
			}
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
					dl.Load(strconv.Itoa(rand.Int()))
				}
				wg.Done()
			}()
		}
		wg.Wait()
	})
}

// BenchmarkLoader benchmark the standard loader
func BenchmarkLoaderExternalCache(b *testing.B) {
	var mu sync.Mutex
	externalCache := make(map[string]*User)
	dl := NewUserLoader(UserLoaderConfig{
		Wait:     500 * time.Nanosecond,
		MaxBatch: 100,
		HookExternalCacheGet: func(key string) (*User, bool) {
			mu.Lock()
			defer mu.Unlock()
			u, ok := externalCache[key]
			return u, ok
		},
		HookExternalCacheSet: func(key string, value *User) error {
			mu.Lock()
			defer mu.Unlock()
			externalCache[key] = value
			return nil
		},
		HookExternalCacheDelete: func(key string) error {
			mu.Lock()
			defer mu.Unlock()
			delete(externalCache, key)
			return nil
		},
		HookExternalCacheClearAll: func() error {
			mu.Lock()
			defer mu.Unlock()
			externalCache = make(map[string]*User)
			return nil
		},
		Fetch: func(keys []string) ([]*User, []error) {
			users := make([]*User, len(keys))
			errors := make([]error, len(keys))

			for i, key := range keys {
				if rand.Int()%100 == 1 {
					errors[i] = fmt.Errorf("user not found")
				} else if rand.Int()%100 == 1 {
					users[i] = nil
				} else {
					users[i] = &User{ID: key, Name: "user " + key}
				}
			}
			return users, errors
		},
	})

	b.Run("caches", func(b *testing.B) {
		thunks := make([]func() (*User, error), b.N)
		for i := 0; i < b.N; i++ {
			_, thunks[i] = dl.LoadThunk(strconv.Itoa(rand.Int() % 300))
		}

		for i := 0; i < b.N; i++ {
			if thunks[i] != nil {
				thunks[i]()
			}
		}
	})

	b.Run("random spread", func(b *testing.B) {
		thunks := make([]func() (*User, error), b.N)
		for i := 0; i < b.N; i++ {
			_, thunks[i] = dl.LoadThunk(strconv.Itoa(rand.Int()))
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
					dl.Load(strconv.Itoa(rand.Int()))
				}
				wg.Done()
			}()
		}
		wg.Wait()
	})
}
