// Code generated by github.com/viocle/dataloaden, DO NOT EDIT.

package example

import (
	"sync"
	"time"
)

// UserByIDAndOrgLoaderConfig captures the config to create a new UserByIDAndOrgLoader
type UserByIDAndOrgLoaderConfig struct {
	// Fetch is a method that provides the data for the loader
	Fetch func(keys []UserByIDAndOrg) ([]*User, []error)

	// Wait is how long to wait before sending a batch
	Wait time.Duration

	// MaxBatch will limit the maximum number of keys to send in one batch, 0 = no limit
	MaxBatch int

	// HookExternalCacheGet is a method that provides the ability to lookup a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	// If the key is found in the external cache, the value should be returned along with true.
	// If the key is not found in the external cache, an empty/nil value should be returned along with false.
	// Both HookExternalCacheGet, HookExternalCacheSet, HookExternalCacheDelete, and HookExternalCacheClearAll should be set if using an external cache.
	HookExternalCacheGet func(key UserByIDAndOrg) (*User, bool)

	// HookExternalCacheSet is a method that provides the ability to set a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	HookExternalCacheSet func(key UserByIDAndOrg, value *User) error

	// HookBeforeFetch is a method that provides the ability to delete/clear a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	HookExternalCacheDelete func(key UserByIDAndOrg) error

	// HookExternalCacheClearAll is a method that provides the ability to clear all keys in an external cache with an external hook.
	HookExternalCacheClearAll func() error

	// HookBeforeFetch is called right before a fetch is performed
	HookBeforeFetch func(keys []UserByIDAndOrg, loaderName string)

	// HookAfterFetch is called right after a fetch is performed
	HookAfterFetch func(keys []UserByIDAndOrg, loaderName string)

	// HookAfterSet is called after a value is set in the cache
	HookAfterSet func(key UserByIDAndOrg, value *User)

	// HookAfterClear is called after a value is cleared from the cache
	HookAfterClear func(key UserByIDAndOrg)

	// HookAfterClearAll is called after all values are cleared from the cache
	HookAfterClearAll func()

	// HookAfterExpired is called after a value is cleared in the cache due to expiration
	HookAfterExpired func(key UserByIDAndOrg)
}

// NewUserByIDAndOrgLoader creates a new UserByIDAndOrgLoader given a fetch, wait, and maxBatch
func NewUserByIDAndOrgLoader(config UserByIDAndOrgLoaderConfig) *UserByIDAndOrgLoader {
	l := &UserByIDAndOrgLoader{
		fetch:                     config.Fetch,
		wait:                      config.Wait,
		maxBatch:                  config.MaxBatch,
		hookExternalCacheGet:      config.HookExternalCacheGet,
		hookExternalCacheSet:      config.HookExternalCacheSet,
		hookExternalCacheDelete:   config.HookExternalCacheDelete,
		hookExternalCacheClearAll: config.HookExternalCacheClearAll,
		hookBeforeFetch:           config.HookBeforeFetch,
		hookAfterFetch:            config.HookAfterFetch,
		hookAfterSet:              config.HookAfterSet,
		hookAfterClear:            config.HookAfterClear,
		hookAfterClearAll:         config.HookAfterClearAll,
		hookAfterExpired:          config.HookAfterExpired,
	}
	l.batchPool = sync.Pool{
		New: func() interface{} {
			return l.createNewBatch()
		},
	}
	l.unsafeBatchSet()
	return l
}

// UserByIDAndOrgLoader batches and caches requests
type UserByIDAndOrgLoader struct {
	// this method provides the data for the loader
	fetch func(keys []UserByIDAndOrg) ([]*User, []error)

	// lazily created cache

	cache map[UserByIDAndOrg]*User

	// the current batch. keys will continue to be collected until timeout is hit,
	// then everything will be sent to the fetch method and out to the listeners
	batch *userByIDAndOrgLoaderBatch

	// how long to done before sending a batch
	wait time.Duration

	// this will limit the maximum number of keys to send in one batch, 0 = no limit
	maxBatch int

	// mutex to prevent races
	mu sync.Mutex

	// hookExternalCacheGet is a method that provides the ability to lookup a key in an external cache with an external hook.
	// If the key is found in the external cache, the value should be returned along with true.
	// If the key is not found in the external cache, an empty/nil value should be returned along with false.
	hookExternalCacheGet func(key UserByIDAndOrg) (*User, bool)

	// hookExternalCacheSet is a method that provides the ability to set a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	hookExternalCacheSet func(key UserByIDAndOrg, value *User) error

	// hookBeforeFetch is a method that provides the ability to delete/clear a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	hookExternalCacheDelete func(key UserByIDAndOrg) error

	// hookExternalCacheClearAll is a method that provides the ability to clear all keys in an external cache with an external hook.
	hookExternalCacheClearAll func() error

	// HookBeforeFetch is called right before a fetch is performed
	hookBeforeFetch func(keys []UserByIDAndOrg, loaderName string)

	// HookAfterFetch is called right after a fetch is performed
	hookAfterFetch func(keys []UserByIDAndOrg, loaderName string)

	// HookAfterSet is called after a value is primed in the cache
	hookAfterSet func(key UserByIDAndOrg, value *User)

	// HookAfterClear is called after a value is cleared from the cache
	hookAfterClear func(key UserByIDAndOrg)

	// HookAfterClearAll is called after all values are cleared from the cache
	hookAfterClearAll func()

	// HookAfterExpired is called after a value is cleared in the cache due to expiration
	hookAfterExpired func(key UserByIDAndOrg)

	// pool of batches
	batchPool sync.Pool
}

type userByIDAndOrgLoaderBatch struct {
	now     int64
	done    chan struct{}
	keysMap map[UserByIDAndOrg]int
	keys    []UserByIDAndOrg
	data    []*User
	errors  []error
	closing bool
}

// Load a User by key, batching and caching will be applied automatically
func (l *UserByIDAndOrgLoader) Load(key UserByIDAndOrg) (*User, error) {
	v, f := l.LoadThunk(key)
	if f != nil {
		return f()
	}
	return v, nil
}

// unsafeBatchSet creates a new batch if one does not exist, otherwise it will reuse the existing batch
func (l *UserByIDAndOrgLoader) unsafeBatchSet() {
	if l.batch == nil {
		b := l.batchPool.Get().(*userByIDAndOrgLoaderBatch)
		// reset
		clear(b.keysMap)
		clear(b.keys)
		l.batch = &userByIDAndOrgLoaderBatch{now: 0, done: make(chan struct{}), keysMap: b.keysMap, keys: b.keys[:0], data: nil, errors: nil}
	} else if l.batch.now == 0 {
		// have a batch but first use, set the start time
		l.batch.now = time.Now().UnixNano()
	}
}

// createNewBatch creates a new batch
func (l *UserByIDAndOrgLoader) createNewBatch() *userByIDAndOrgLoaderBatch {
	return &userByIDAndOrgLoaderBatch{now: 0, done: make(chan struct{}), keysMap: make(map[UserByIDAndOrg]int, l.maxBatch), keys: make([]UserByIDAndOrg, 0, l.maxBatch), data: nil, errors: nil}
}

// LoadThunk returns a function that when called will block waiting for a User.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *UserByIDAndOrgLoader) LoadThunk(key UserByIDAndOrg) (*User, func() (*User, error)) {
	if l.hookExternalCacheGet != nil {
		if v, ok := l.hookExternalCacheGet(key); ok {
			return v, nil
		}
		// not found in external cache, continue
		l.mu.Lock()
		l.unsafeBatchSet()
	} else {
		l.mu.Lock()

		if len(l.cache) > 0 {
			if it, ok := l.cache[key]; ok {
				l.mu.Unlock()
				return it, nil
			}
		}
		l.unsafeBatchSet()

	}
	batch := l.batch
	pos := batch.keyIndex(l, key)
	l.mu.Unlock()

	return nil, func() (*User, error) {
		<-batch.done

		var data *User
		if pos < len(batch.data) {
			data = batch.data[pos]
		}

		var err error
		// its convenient to be able to return a single error for everything
		if len(batch.errors) == 1 {
			err = batch.errors[0]
		} else if batch.errors != nil {
			err = batch.errors[pos]
		}

		if err == nil {
			l.mu.Lock()
			l.unsafeSet(key, data)
			l.mu.Unlock()
		}

		return data, err
	}
}

// LoadAll fetches many keys at once. It will be broken into appropriate sized
// sub batches depending on how the loader is configured
func (l *UserByIDAndOrgLoader) LoadAll(keys []UserByIDAndOrg) ([]*User, []error) {
	users := make([]*User, len(keys))
	thunks := make(map[int]func() (*User, error), len(keys))
	errors := make([]error, len(keys))

	for i, key := range keys {
		if v, thunk := l.LoadThunk(key); thunk != nil {
			thunks[i] = thunk
		} else {
			users[i] = v
		}
	}
	for i, thunk := range thunks {
		users[i], errors[i] = thunk()
	}

	return users, errors
}

// LoadAllThunk returns a function that when called will block waiting for a Users.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *UserByIDAndOrgLoader) LoadAllThunk(keys []UserByIDAndOrg) func() ([]*User, []error) {
	thunks := make(map[int]func() (*User, error), len(keys))
	users := make([]*User, len(keys))
	for i, key := range keys {
		if v, thunk := l.LoadThunk(key); thunk != nil {
			thunks[i] = thunk
		} else {
			users[i] = v
		}
	}
	return func() ([]*User, []error) {
		errors := make([]error, len(keys))
		for i, thunk := range thunks {
			users[i], errors[i] = thunk()
		}
		return users, errors
	}
}

// unsafePrime will prime the cache with the given key and value if the key does not exist. This method is not thread safe.
func (l *UserByIDAndOrgLoader) unsafePrime(key UserByIDAndOrg, value *User, forceReplace bool) bool {
	if l.hookExternalCacheSet != nil {
		// make a copy when writing to the cache, its easy to pass a pointer in from a loop var
		// and end up with the whole cache pointing to the same value.
		cpy := *value
		if err := l.hookExternalCacheSet(key, &cpy); err != nil {
			return false
		}
		if l.hookAfterSet != nil {
			l.hookAfterSet(key, value)
		}
		return true
	}
	var found bool

	if _, found = l.cache[key]; found && forceReplace {
		delete(l.cache, key)
	}
	if !found || forceReplace {
		// make a copy when writing to the cache, its easy to pass a pointer in from a loop var
		// and end up with the whole cache pointing to the same value.
		cpy := *value
		l.unsafeSet(key, &cpy)
	}

	return !found || forceReplace
}

// PrimeMany will prime the cache with the given keys and values. Value index is matched to key index.
func (l *UserByIDAndOrgLoader) PrimeMany(keys []UserByIDAndOrg, values []*User) []bool {
	if len(keys) != len(values) {
		// keys and values must be the same length
		return make([]bool, len(keys))
	}
	ret := make([]bool, len(keys))
	l.mu.Lock()
	for i, key := range keys {
		ret[i] = l.unsafePrime(key, values[i], false)
	}
	l.mu.Unlock()
	return ret
}

// Prime the cache with the provided key and value. If the key already exists, no change is made
// and false is returned.
// (To forcefully prime the cache, clear the key first with loader.clear(key).prime(key, value).)
func (l *UserByIDAndOrgLoader) Prime(key UserByIDAndOrg, value *User) bool {
	l.mu.Lock()
	found := l.unsafePrime(key, value, false)
	l.mu.Unlock()
	return found
}

// ForcePrime the cache with the provided key and value. If the key already exists, value is replaced
// (This removes the requirement to clear the key first with loader.clear(key).prime(key, value))
func (l *UserByIDAndOrgLoader) ForcePrime(key UserByIDAndOrg, value *User) {
	l.mu.Lock()
	l.unsafePrime(key, value, true)
	l.mu.Unlock()
}

// Clear the value at key from the cache, if it exists
func (l *UserByIDAndOrgLoader) Clear(key UserByIDAndOrg) {
	if l.hookExternalCacheDelete != nil {
		l.hookExternalCacheDelete(key)
		if l.hookAfterClear != nil {
			l.hookAfterClear(key)
		}
		return
	}

	l.mu.Lock()
	delete(l.cache, key)
	l.mu.Unlock()

	if l.hookAfterClear != nil {
		l.hookAfterClear(key)
	}
}

// ClearAll clears all values from the cache
func (l *UserByIDAndOrgLoader) ClearAll() {
	if l.hookExternalCacheClearAll != nil {
		l.hookExternalCacheClearAll()
		if l.hookAfterClearAll != nil {
			l.hookAfterClearAll()
		}
		return
	}

	l.mu.Lock()
	l.cache = make(map[UserByIDAndOrg]*User, l.maxBatch)
	l.mu.Unlock()

	if l.hookAfterClearAll != nil {
		l.hookAfterClearAll()
	}
}

// unsafeSet will set the key to value without any locks or checks. This method is not thread safe.
func (l *UserByIDAndOrgLoader) unsafeSet(key UserByIDAndOrg, value *User) {
	if l.hookExternalCacheSet != nil {
		l.hookExternalCacheSet(key, value)
		if l.hookAfterSet != nil {
			l.hookAfterSet(key, value)
		}
		return
	}

	if l.cache == nil {
		l.cache = make(map[UserByIDAndOrg]*User, l.maxBatch)
	}
	l.cache[key] = value

	if l.hookAfterSet != nil {
		l.hookAfterSet(key, value)
	}
}

// keyIndex will return the location of the key in the batch, if its not found
// it will add the key to the batch
func (b *userByIDAndOrgLoaderBatch) keyIndex(l *UserByIDAndOrgLoader, key UserByIDAndOrg) int {
	if i, ok := b.keysMap[key]; ok {
		return i
	}

	pos := len(b.keysMap)
	b.keysMap[key] = pos
	b.keys = append(b.keys, key)
	if pos == 0 {
		go b.startTimer(l)
	}

	if l.maxBatch != 0 && pos >= l.maxBatch-1 {
		if !b.closing {
			b.closing = true
			l.batch = nil
			go b.end(l)
		}
	}

	return pos
}

// startTimer will wait the desired wait time before sending the batch unless another batch limit had been reached
func (b *userByIDAndOrgLoaderBatch) startTimer(l *UserByIDAndOrgLoader) {
	time.Sleep(l.wait)
	l.mu.Lock()

	// we must have hit a batch limit and are already finalizing this batch
	if b.closing {
		l.mu.Unlock()
		return
	}

	l.batch = nil
	l.mu.Unlock()

	b.end(l)
}

// end calls fetch and closes the done channel to unblock all thunks
func (b *userByIDAndOrgLoaderBatch) end(l *UserByIDAndOrgLoader) {
	if l.hookBeforeFetch != nil {
		l.hookBeforeFetch(b.keys, "UserByIDAndOrgLoader")
	}
	b.data, b.errors = l.fetch(b.keys)
	if l.hookAfterFetch != nil {
		l.hookAfterFetch(b.keys, "UserByIDAndOrgLoader")
	}
	close(b.done)
}
