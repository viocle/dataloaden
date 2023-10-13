package slice

import (
	"testing"
	"time"

	"github.com/viocle/dataloaden/example"
)

func TestUserSliceLoader(t *testing.T) {
	// create new User slice loader without cache expiration
	loader := NewLoader()

	// prime 3 users to the cache
	if !loader.Prime(1, []example.User{{ID: "1", Name: "user 1"}}) {
		t.Error("Failed to prime user 1 to the cache")
	}
	if !loader.Prime(2, []example.User{{ID: "2", Name: "user 2"}}) {
		t.Error("Failed to prime user 2 to the cache")
	}
	if !loader.Prime(3, []example.User{{ID: "3", Name: "user 3"}}) {
		t.Error("Failed to prime user 3 to the cache")
	}

	// confirm loader cache contains 3 users
	if len(loader.cache) != 3 {
		t.Errorf("Expected 3 users in the cache, got %d", len(loader.cache))
	}

	// load user 1 from the cache
	if user, err := loader.Load(1); err != nil {
		t.Errorf("Failed to load user 1: %v", err)
	} else if len(user) != 1 {
		t.Errorf("Expected 1 user, got %d", len(user))
	} else if user[0].ID != "1" {
		t.Errorf("Expected user 1, got %s", user[0].ID)
	}

	// load user 1 and user 2 from the cache
	if users, _ := loader.LoadAll([]int{1, 2}); len(users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(users))
	} else {
		for _, user := range users {
			if len(user) != 1 {
				t.Errorf("Expected 1 user, got %d", len(user))
			} else if user[0].ID != "1" && user[0].ID != "2" {
				t.Errorf("Expected user 1 or 2, got %s", user[0].ID)
			}
		}
	}

	// clear user 1 from the cache
	loader.Clear(1)

	// confirm user 1 is not in the cache
	if user, ok := loader.cache[1]; ok || len(user) != 0 {
		t.Errorf("Expected user 1 to be cleared from the cache, got %v", user)
	}

	// load user 3 from the cache
	if user, err := loader.Load(3); err != nil {
		t.Errorf("Failed to load user 3: %v", err)
	} else if len(user) != 1 {
		t.Errorf("Expected 1 user when loading user 3, got %d", len(user))
	} else if user[0].ID != "3" {
		t.Errorf("Expected user 3, got %s", user[0].ID)
	}

	// load user 4 from the cache, letting fetch set the value
	if user, err := loader.Load(4); err != nil {
		t.Errorf("Failed to load user 4: %v", err)
	} else if len(user) != 1 {
		t.Errorf("Expected 1 user when loading user 4, got %d", len(user))
	} else if user[0].ID != "4" {
		t.Errorf("Expected user 4, got %s", user[0].ID)
	}

	// clear all users from the cache
	loader.ClearAll()

	// confirm no users exist in the cache
	if len(loader.cache) != 0 {
		t.Errorf("Expected cache to be cleared, got %v", loader.cache)
	}

	// set loader cache expiration to 100ms to change to using cacheExpire in the background
	// this should never be done in practice, but is done here to test the functionality
	loader.expireAfter = (time.Millisecond * 100).Nanoseconds()

	// prime user 1 to the cache
	if !loader.Prime(1, []example.User{{ID: "1", Name: "user 1"}}) {
		t.Error("Failed to prime user 1 to the cache")
	}

	// confirm user 1 is in cacheExpire
	if user, ok := loader.cacheExpire[1]; !ok || user == nil {
		t.Errorf("Expected user 1 to be in cacheExpire, got %v", user)
	} else if len(user.Value) != 1 {
		t.Errorf("Expected 1 user in cacheExpire value, got %d", len(user.Value))
	} else if user.Value[0].ID != "1" {
		t.Errorf("Expected user 1 in cacheExpire value, got %s", user.Value[0].ID)
	}

	// load user 1 from the cache
	if user, err := loader.Load(1); err != nil {
		t.Errorf("Failed to load user 1: %v", err)
	} else if len(user) != 1 {
		t.Errorf("Expected 1 user, got %d", len(user))
	} else if user[0].ID != "1" {
		t.Errorf("Expected user 1, got %s", user[0].ID)
	}

	// wait for cache to expire
	time.Sleep(time.Millisecond * 101)

	// confirm user 1 is in cacheExpire but is expired
	if user, ok := loader.cacheExpire[1]; !ok || user == nil {
		t.Errorf("Expected user 1 to be in cacheExpire, got %v", user)
	} else if len(user.Value) != 1 {
		t.Errorf("Expected 1 user in cacheExpire value, got %d", len(user.Value))
	} else if user.Value[0].ID != "1" {
		t.Errorf("Expected user 1 in cacheExpire value, got %s", user.Value[0].ID)
	} else if !user.expired(time.Now().UnixNano()) {
		t.Errorf("Expected user 1 to be expired, got %v", user)
	}

	// clear all expired cache items
	loader.ClearExpired()

	// confirm user 1 is not in cacheExpire
	if user, ok := loader.cacheExpire[1]; ok || user != nil {
		t.Errorf("Expected user 1 to be cleared from cacheExpire, got %v", user)
	} else if len(loader.cacheExpire) != 0 {
		t.Errorf("Expected cacheExpire to be cleared, got %v", loader.cacheExpire)
	}

	// prime user 1 and user 2 to the cache
	if results := loader.PrimeMany([]int{1, 2}, [][]example.User{{{ID: "1", Name: "user 1"}}, {{ID: "2", Name: "user 2"}}}); len(results) != 2 {
		t.Errorf("Expected 2 prime results, got %d", len(results))
	} else {
		for _, result := range results {
			if !result {
				t.Errorf("Expected PrimeMany result to be true, got %v", results)
			}
		}
	}

	// load user 1 from the cache
	if user, err := loader.Load(1); err != nil {
		t.Errorf("Failed to load user 1: %v", err)
	} else if len(user) != 1 {
		t.Errorf("Expected 1 user, got %d", len(user))
	} else if user[0].ID != "1" {
		t.Errorf("Expected user 1, got %s", user[0].ID)
	}

	// ForcePrime cache with User 1 but change the name to User 1 Updated
	loader.ForcePrime(1, []example.User{{ID: "1", Name: "user 1 updated"}})

	// load user 1 from the cache and verify the name is User 1 Updated
	if user, err := loader.Load(1); err != nil {
		t.Errorf("Failed to load user 1: %v", err)
	} else if len(user) != 1 {
		t.Errorf("Expected 1 user, got %d", len(user))
	} else if user[0].ID != "1" {
		t.Errorf("Expected user 1, got %s", user[0].ID)
	} else if user[0].Name != "user 1 updated" {
		t.Errorf("Expected user 1 name to be user 1 updated, got %s", user[0].Name)
	}
}
