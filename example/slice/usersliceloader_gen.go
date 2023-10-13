// Code generated by github.com/viocle/dataloaden, DO NOT EDIT.

package slice

import (
	"sync"
	"time"

	"github.com/viocle/dataloaden/example"
)

// UserSliceLoaderConfig captures the config to create a new UserSliceLoader
type UserSliceLoaderConfig struct {
	// Fetch is a method that provides the data for the loader
	Fetch func(keys []int) ([][]example.User, []error)

	// Wait is how long to wait before sending a batch
	Wait time.Duration

	// MaxBatch will limit the maximum number of keys to send in one batch, 0 = no limit
	MaxBatch int

	// ExpireAfter determines how long until cached items expire. Set to 0 to disable expiration
	ExpireAfter time.Duration

	// TriggerAfterSet is called after a value is set in the cache
	TriggerAfterSet func(key int, value []example.User)

	// TriggerAfterClear is called after a value is cleared from the cache
	TriggerAfterClear func(key int)

	// TriggerAfterClearAll is called after all values are cleared from the cache
	TriggerAfterClearAll func()

	// TriggerAfterExpired is called after a value is cleared in the cache due to expiration
	TriggerAfterExpired func(key int)
}

// UserSliceLoaderCacheItem defines a cache item when using dataloader cache expiration where expireAfter > 0
type UserSliceLoaderCacheItem struct {
	// Expires contains the time this CacheItem expires
	Expires int64

	// Value contains the cached []example.User
	Value []example.User
}

// expired returns true if the cache item has expired
func (c *UserSliceLoaderCacheItem) expired(now int64) bool {
	return c.Expires < now
}

// NewUserSliceLoader creates a new UserSliceLoader given a fetch, wait, and maxBatch
func NewUserSliceLoader(config UserSliceLoaderConfig) *UserSliceLoader {
	l := &UserSliceLoader{
		fetch:    config.Fetch,
		wait:     config.Wait,
		maxBatch: config.MaxBatch,

		expireAfter: config.ExpireAfter.Nanoseconds(),

		triggerAfterSet:      config.TriggerAfterSet,
		triggerAfterClear:    config.TriggerAfterClear,
		triggerAfterClearAll: config.TriggerAfterClearAll,
		triggerAfterExpired:  config.TriggerAfterExpired,
	}
	l.batchPool = sync.Pool{
		New: func() interface{} {
			return l.createNewBatch()
		},
	}
	l.unsafeBatchSet()
	return l
}

// UserSliceLoader batches and caches requests
type UserSliceLoader struct {
	// this method provides the data for the loader
	fetch func(keys []int) ([][]example.User, []error)

	// lazily created cache

	cacheExpire map[int]*UserSliceLoaderCacheItem

	cache map[int][]example.User

	// the current batch. keys will continue to be collected until timeout is hit,
	// then everything will be sent to the fetch method and out to the listeners
	batch *userSliceLoaderBatch

	// how long to done before sending a batch
	wait time.Duration

	// this will limit the maximum number of keys to send in one batch, 0 = no limit
	maxBatch int

	// the amount of nanoseconds a cache item should remain valid. This will determine if cache expiration will be used, 0 = no expiration
	expireAfter int64

	// mutex to prevent races
	mu sync.Mutex

	// triggerAfterSet is called after a value is primed in the cache
	triggerAfterSet func(key int, value []example.User)

	// triggerAfterClear is called after a value is cleared from the cache
	triggerAfterClear func(key int)

	// triggerAfterClearAll is called after all values are cleared from the cache
	triggerAfterClearAll func()

	// triggerAfterExpired is called after a value is cleared in the cache due to expiration
	triggerAfterExpired func(key int)

	// pool of batches
	batchPool sync.Pool
}

type userSliceLoaderBatch struct {
	now     int64
	done    chan struct{}
	keysMap map[int]int
	keys    []int
	data    [][]example.User
	errors  []error
	closing bool
}

// Load a User by key, batching and caching will be applied automatically
func (l *UserSliceLoader) Load(key int) ([]example.User, error) {
	v, f := l.LoadThunk(key)
	if f != nil {
		return f()
	}
	return v, nil
}

// unsafeBatchSet creates a new batch if one does not exist, otherwise it will reuse the existing batch
func (l *UserSliceLoader) unsafeBatchSet() {
	if l.batch == nil {
		b := l.batchPool.Get().(*userSliceLoaderBatch)
		// reset
		clear(b.keysMap)
		clear(b.keys)
		b.keys = b.keys[:0]
		l.batch = &userSliceLoaderBatch{now: 0, done: make(chan struct{}), keysMap: b.keysMap, keys: b.keys[:0], data: nil, errors: nil}
	} else if l.batch.now == 0 {
		// have a batch but first use, set the start time
		l.batch.now = time.Now().UnixNano()
	}
}

// createNewBatch creates a new batch
func (l *UserSliceLoader) createNewBatch() *userSliceLoaderBatch {
	return &userSliceLoaderBatch{now: 0, done: make(chan struct{}), keysMap: make(map[int]int, l.maxBatch), keys: make([]int, 0, l.maxBatch), data: nil, errors: nil}
}

// LoadThunk returns a function that when called will block waiting for a User.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *UserSliceLoader) LoadThunk(key int) ([]example.User, func() ([]example.User, error)) {
	l.mu.Lock()

	if l.expireAfter <= 0 && len(l.cache) > 0 {
		// not using cache expiration
		if it, ok := l.cache[key]; ok {
			l.mu.Unlock()
			return it, nil
		}
		l.unsafeBatchSet()
	} else if l.expireAfter > 0 && len(l.cacheExpire) > 0 {
		// using cache expiration
		l.unsafeBatchSet()
		if it, ok := l.cacheExpire[key]; ok {
			if it != nil && !it.expired(l.batch.now) {
				l.mu.Unlock()
				return it.Value, nil
			}
			// cache item has expired, clear from cache
			delete(l.cacheExpire, key)
			if l.triggerAfterExpired != nil {
				go l.triggerAfterExpired(key)
			}
		}
	} else {
		// no cache
		l.unsafeBatchSet()
	}

	batch := l.batch
	pos := batch.keyIndex(l, key)
	l.mu.Unlock()

	return nil, func() ([]example.User, error) {
		<-batch.done

		var data []example.User
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
func (l *UserSliceLoader) LoadAll(keys []int) ([][]example.User, []error) {
	users := make([][]example.User, len(keys))
	thunks := make(map[int]func() ([]example.User, error), len(keys))
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
func (l *UserSliceLoader) LoadAllThunk(keys []int) func() ([][]example.User, []error) {
	thunks := make(map[int]func() ([]example.User, error), len(keys))
	users := make([][]example.User, len(keys))
	for i, key := range keys {
		if v, thunk := l.LoadThunk(key); thunk != nil {
			thunks[i] = thunk
		} else {
			users[i] = v
		}
	}
	return func() ([][]example.User, []error) {
		errors := make([]error, len(keys))
		for i, thunk := range thunks {
			users[i], errors[i] = thunk()
		}
		return users, errors
	}
}

// unsafePrime will prime the cache with the given key and value if the key does not exist. This method is not thread safe.
func (l *UserSliceLoader) unsafePrime(key int, value []example.User, forceReplace bool) bool {
	var found bool

	if l.expireAfter <= 0 {
		// not using cache expiration

		if _, found = l.cache[key]; found && forceReplace {
			delete(l.cache, key)
		}
		if !found || forceReplace {
			// make a copy when writing to the cache, its easy to pass a pointer in from a loop var
			// and end up with the whole cache pointing to the same value.
			cpy := make([]example.User, len(value))
			copy(cpy, value)
			l.unsafeSet(key, cpy)
		}

	} else {
		// using cache expiration
		if _, found = l.cacheExpire[key]; found && forceReplace {
			delete(l.cacheExpire, key)
		}
		if !found || forceReplace {
			// make a copy when writing to the cache, its easy to pass a pointer in from a loop var
			// and end up with the whole cache pointing to the same value.
			cpy := make([]example.User, len(value))
			copy(cpy, value)
			l.unsafeSet(key, cpy)
		}
	}

	return !found || forceReplace
}

// PrimeMany will prime the cache with the given keys and values. Value index is matched to key index.
func (l *UserSliceLoader) PrimeMany(keys []int, values [][]example.User) []bool {
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
func (l *UserSliceLoader) Prime(key int, value []example.User) bool {
	l.mu.Lock()
	found := l.unsafePrime(key, value, false)
	l.mu.Unlock()
	return found
}

// ForcePrime the cache with the provided key and value. If the key already exists, value is replaced
// (This removes the requirement to clear the key first with loader.clear(key).prime(key, value))
func (l *UserSliceLoader) ForcePrime(key int, value []example.User) {
	l.mu.Lock()
	l.unsafePrime(key, value, true)
	l.mu.Unlock()
}

// Clear the value at key from the cache, if it exists
func (l *UserSliceLoader) Clear(key int) {

	if l.expireAfter <= 0 {
		// not using cache expiration

		l.mu.Lock()
		delete(l.cache, key)
		l.mu.Unlock()

	} else {
		// using cache expiration
		l.mu.Lock()
		delete(l.cacheExpire, key)
		l.mu.Unlock()
	}

	if l.triggerAfterClear != nil {
		go l.triggerAfterClear(key)
	}
}

// ClearAll clears all values from the cache
func (l *UserSliceLoader) ClearAll() {

	if l.expireAfter <= 0 {
		// not using cache expiration

		l.mu.Lock()
		l.cache = make(map[int][]example.User, l.maxBatch)
		l.mu.Unlock()

	} else {
		// using cache expiration
		l.mu.Lock()
		l.cacheExpire = make(map[int]*UserSliceLoaderCacheItem, l.maxBatch)
		l.mu.Unlock()
	}

	if l.triggerAfterClearAll != nil {
		go l.triggerAfterClearAll()
	}
}

// ClearExpired clears all expired values from the cache if cache expiration is being used
func (l *UserSliceLoader) ClearExpired() {
	if l.expireAfter > 0 {
		// using cache expiration
		tNow := time.Now().UnixNano()
		l.mu.Lock()
		for cacheKey, cacheItem := range l.cacheExpire {
			if cacheItem != nil && tNow > cacheItem.Expires {
				// value has expired
				delete(l.cacheExpire, cacheKey)
				if l.triggerAfterExpired != nil {
					go l.triggerAfterExpired(cacheKey)
				}
			}
		}
		l.mu.Unlock()
	}
}

// unsafeSet will set the key to value without any locks or checks. This method is not thread safe.
func (l *UserSliceLoader) unsafeSet(key int, value []example.User) {

	if l.expireAfter <= 0 {
		// not using cache expiration

		if l.cache == nil {
			l.cache = make(map[int][]example.User, l.maxBatch)
		}
		l.cache[key] = value

	} else {
		// using cache expiration
		if l.cacheExpire == nil {
			l.cacheExpire = make(map[int]*UserSliceLoaderCacheItem, l.maxBatch)
		}
		l.cacheExpire[key] = &UserSliceLoaderCacheItem{Expires: time.Now().UnixNano() + l.expireAfter, Value: value}
	}

	if l.triggerAfterSet != nil {
		go l.triggerAfterSet(key, value)
	}
}

// keyIndex will return the location of the key in the batch, if its not found
// it will add the key to the batch
func (b *userSliceLoaderBatch) keyIndex(l *UserSliceLoader, key int) int {
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
func (b *userSliceLoaderBatch) startTimer(l *UserSliceLoader) {
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
func (b *userSliceLoaderBatch) end(l *UserSliceLoader) {
	b.data, b.errors = l.fetch(b.keys)
	close(b.done)
}
