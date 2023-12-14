// Code generated by github.com/viocle/dataloaden, DO NOT EDIT.

package slice

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/viocle/dataloaden/example"
)

const (
	UserSliceLoaderCacheKeyPrefix = "{DataLoaderUserSliceLoader}:"
)

var (
	ErrUserSliceLoaderGetManyLength = errors.New("redis error, invalid length returned from GetManyFunc")
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

	// HookExternalCacheGet is a method that provides the ability to lookup a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	// If the key is found in the external cache, the value should be returned along with true.
	// If the key is not found in the external cache, an empty/nil value should be returned along with false.
	// Both HookExternalCacheGet, HookExternalCacheSet, HookExternalCacheDelete, and HookExternalCacheClearAll should be set if using an external cache.
	HookExternalCacheGet func(key int) ([]example.User, bool)

	// HookExternalCacheSet is a method that provides the ability to set a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	HookExternalCacheSet func(key int, value []example.User) error

	// HookBeforeFetch is a method that provides the ability to delete/clear a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	HookExternalCacheDelete func(key int) error

	// HookExternalCacheClearAll is a method that provides the ability to clear all keys in an external cache with an external hook.
	HookExternalCacheClearAll func() error

	// HookBeforeFetch is called right before a fetch is performed
	HookBeforeFetch func(keys []int, loaderName string)

	// HookAfterFetch is called right after a fetch is performed
	HookAfterFetch func(keys []int, loaderName string)

	// HookAfterSet is called after a value is set in the cache
	HookAfterSet func(key int, value []example.User)

	// HookAfterPrime is called after a value is primed in the cache using Prime or ForcePrime
	HookAfterPrime func(key int, value []example.User)

	// HookAfterClear is called after a value is cleared from the cache
	HookAfterClear func(key int)

	// HookAfterClearAll is called after all values are cleared from the cache
	HookAfterClearAll func()

	// HookAfterExpired is called after a value is cleared in the cache due to expiration
	HookAfterExpired func(key int)

	// RedisConfig is used to configure a UserSliceLoader backed by Redis, disabling the internal cache.
	RedisConfig *UserSliceLoaderRedisConfig
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
		fetch:                     config.Fetch,
		wait:                      config.Wait,
		maxBatch:                  config.MaxBatch,
		expireAfter:               config.ExpireAfter.Nanoseconds(),
		hookExternalCacheGet:      config.HookExternalCacheGet,
		hookExternalCacheSet:      config.HookExternalCacheSet,
		hookExternalCacheDelete:   config.HookExternalCacheDelete,
		hookExternalCacheClearAll: config.HookExternalCacheClearAll,
		hookBeforeFetch:           config.HookBeforeFetch,
		hookAfterFetch:            config.HookAfterFetch,
		hookAfterSet:              config.HookAfterSet,
		hookAfterPrime:            config.HookAfterPrime,
		hookAfterClear:            config.HookAfterClear,
		hookAfterClearAll:         config.HookAfterClearAll,
		hookAfterExpired:          config.HookAfterExpired,
		redisConfig:               config.RedisConfig,
	}
	if config.RedisConfig != nil {
		// validate we have all the required Redis functions. If not, force disable Redis
		if l.redisConfig.GetFunc != nil && l.redisConfig.SetFunc != nil && l.redisConfig.DeleteFunc != nil {
			// all required Redis functions are present, enable Redis
			l.redisConfig = &UserSliceLoaderRedisConfig{
				SetTTL:          config.RedisConfig.SetTTL,          // optional
				GetFunc:         config.RedisConfig.GetFunc,         // (GET)
				GetManyFunc:     config.RedisConfig.GetManyFunc,     // (MGET) optional, but recommended for LoadAll performance
				SetFunc:         config.RedisConfig.SetFunc,         // (SET)
				DeleteFunc:      config.RedisConfig.DeleteFunc,      // (DEL)
				DeleteManyFunc:  config.RedisConfig.DeleteManyFunc,  // (DEL) optional, but recommened for ClearAll performance
				ObjMarshal:      config.RedisConfig.ObjMarshal,      // optional
				ObjUnmarshal:    config.RedisConfig.ObjUnmarshal,    // optional
				KeyToStringFunc: config.RedisConfig.KeyToStringFunc, // optional, but recommended for complex types that need to be serialized
			}
			if l.redisConfig.ObjMarshal == nil || l.redisConfig.ObjUnmarshal == nil {
				// missing ObjMarshal or ObjUnmarshal, force use of json package
				l.redisConfig.ObjMarshal = json.Marshal
				l.redisConfig.ObjUnmarshal = json.Unmarshal
			}
			// set batchResultSet to just call the SetFunc directly, no locks needed
			l.batchResultSet = func(key int, value []example.User) {
				l.redisConfig.SetFunc(context.Background(), UserSliceLoaderCacheKeyPrefix+strconv.FormatInt(int64(key), 10), value, l.redisConfig.SetTTL)
			}
			if l.redisConfig.KeyToStringFunc == nil {
				l.redisConfig.KeyToStringFunc = l.MarshalUserSliceLoaderToString
			}
		}
	}
	if l.redisConfig == nil {
		// set the default batchResultSet
		l.batchResultSet = func(key int, value []example.User) {
			l.mu.Lock()
			l.unsafeSet(key, value)
			l.mu.Unlock()
		}
	}
	l.batchPool = sync.Pool{
		New: func() interface{} {
			return l.createNewBatch()
		},
	}
	l.unsafeBatchSet()
	return l
}

// UserSliceLoaderRedisConfig is used to configure a UserSliceLoader backed by Redis. GetFunc, SetFunc, and DeleteFunc are required if using Redis. If any function is not provided, Redis will be disabled and internal caching will be used.
type UserSliceLoaderRedisConfig struct {
	// SetTTL is the TTL (Time To Live) for a key to live in Redis on set. If nil, no TTL will be set.
	SetTTL *time.Duration

	// GetFunc should get a value from Redis given a key and return the raw string value.
	GetFunc func(ctx context.Context, key string) (string, error)

	// GetManyFunc should get one or more values from Redis given a set of keys and return the raw string values, errors the size of keys with non nil values for keys not found, and an error if any other error occurred running the command
	// If not set then GetFunc will be used instead, but will be called one at a time for each key
	GetManyFunc func(ctx context.Context, keys []string) ([]string, []error, error)

	// SetFunc should set a value in Redis given a key and value with an optional ttl (Time To Live)
	SetFunc func(ctx context.Context, key string, value interface{}, ttl *time.Duration) error

	// DeleteFunc should delete a value in Redis given a key
	DeleteFunc func(ctx context.Context, key string) error

	// DeleteManyFunc should delete one or more values in Redis given a set of keys
	DeleteManyFunc func(ctx context.Context, key []string) error

	// GetKeysFunc should return all keys in Redis matching the given pattern. If not set then ClearAll() for this dataloader will not be supported.
	GetKeysFunc func(ctx context.Context, pattern string) ([]string, error)

	// ObjMarshal provides you the ability to specify your own encoding package. If not set, the default encoding/json package will be used.
	ObjMarshal func(any) ([]byte, error)

	// ObjUnmarshaler provides you the ability to specify your own encoding package. If not set, the default encoding/json package will be used.
	ObjUnmarshal func([]byte, any) error

	// KeyToStringFunc provides you the ability to specify your own function to convert a key to a string, which will be used instead of serialization.
	// This is only used for non standard types that need to be serialized. If not set, the ObjMarshal function (user defined or default) will be used to serialize a key into a string value
	// Example: If you have a struct with a String() function that returns a string representation of the struct, you can set this function to that function.
	//
	// type MyStruct struct {
	//     ID string
	//     OrgID string
	// }
	// ...
	// UserSliceLoaderRedisConfig{
	//		KeyToStringFunc = func(key int) string { return m.ID + ":" + m.OrgID }
	// }
	// ...
	// Or if your key type has a String() function that returns a string representation of the key, you can set this function like this:
	// UserSliceLoaderRedisConfig{
	//		KeyToStringFunc = func(key int) string { return key.String() }
	// }
	KeyToStringFunc func(key int) string
}

// UserSliceLoader batches and caches requests
type UserSliceLoader struct {
	// this method provides the data for the loader
	fetch func(keys []int) ([][]example.User, []error)

	// optional Redis configuration
	redisConfig *UserSliceLoaderRedisConfig

	// lazily created cache

	cacheExpire map[int]*UserSliceLoaderCacheItem

	cache map[int][]example.User

	// the current batch. keys will continue to be collected until timeout is hit,
	// then everything will be sent to the fetch method and out to the listeners
	batch *userSliceLoaderBatch

	// batchResultSet sets the batch result
	batchResultSet func(int, []example.User)

	// how long to done before sending a batch
	wait time.Duration

	// this will limit the maximum number of keys to send in one batch, 0 = no limit
	maxBatch int

	// the amount of nanoseconds a cache item should remain valid. This will determine if cache expiration will be used, 0 = no expiration
	expireAfter int64

	// mutex to prevent races
	mu sync.Mutex

	// hookExternalCacheGet is a method that provides the ability to lookup a key in an external cache with an external hook.
	// If the key is found in the external cache, the value should be returned along with true.
	// If the key is not found in the external cache, an empty/nil value should be returned along with false.
	hookExternalCacheGet func(key int) ([]example.User, bool)

	// hookExternalCacheSet is a method that provides the ability to set a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	hookExternalCacheSet func(key int, value []example.User) error

	// hookBeforeFetch is a method that provides the ability to delete/clear a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	hookExternalCacheDelete func(key int) error

	// hookExternalCacheClearAll is a method that provides the ability to clear all keys in an external cache with an external hook.
	hookExternalCacheClearAll func() error

	// hookBeforeFetch is called right before a fetch is performed
	hookBeforeFetch func(keys []int, loaderName string)

	// hookAfterFetch is called right after a fetch is performed
	hookAfterFetch func(keys []int, loaderName string)

	// hookAfterSet is called after a value is set in the cache
	hookAfterSet func(key int, value []example.User)

	// hookAfterPrime is called after a value is primed in the cache using Prime or ForcePrime
	hookAfterPrime func(key int, value []example.User)

	// hookAfterClear is called after a value is cleared from the cache
	hookAfterClear func(key int)

	// hookAfterClearAll is called after all values are cleared from the cache
	hookAfterClearAll func()

	// hookAfterExpired is called after a value is cleared in the cache due to expiration
	hookAfterExpired func(key int)

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
	if l.redisConfig != nil {
		// using Redis
		v, err := l.redisConfig.GetFunc(context.Background(), UserSliceLoaderCacheKeyPrefix+strconv.FormatInt(int64(key), 10))
		if err == nil {
			// found in Redis, attempt to return value
			if v == "" || v == "null" {
				// key found, empty value, return nil
				return nil, nil
			}
			var ret []example.User
			if err := l.redisConfig.ObjUnmarshal([]byte(v), &ret); err == nil {
				return ret, nil
			}
			// error unmarshalling, just add to batch
		}
		// not found in Redis or error, continue
		l.mu.Lock()
		l.unsafeBatchSet()
	} else {
		if l.hookExternalCacheGet != nil {
			if v, ok := l.hookExternalCacheGet(key); ok {
				return v, nil
			}
			// not found in external cache, continue
			l.mu.Lock()
			l.unsafeBatchSet()
		} else {
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
					if l.hookAfterExpired != nil {
						l.hookAfterExpired(key)
					}
				}
			} else {
				// no cache
				l.unsafeBatchSet()
			}

		}
	}
	return l.addToBatchUnsafe(key)
}

// addToBatchUnsafe adds the key to the current batch and returns a thunk to be called later. This method is not thread safe. Expects l.unsafeBatchSet() and l.mu.lock() to have been called prior to calling this method.
func (l *UserSliceLoader) addToBatchUnsafe(key int) ([]example.User, func() ([]example.User, error)) {
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
			l.batchResultSet(key, data)
		}

		return data, err
	}
}

// LoadAll fetches many keys at once. It will be broken into appropriate sized
// sub batches depending on how the loader is configured
func (l *UserSliceLoader) LoadAll(keys []int) ([][]example.User, []error) {
	if len(keys) == 0 {
		return nil, nil
	}
	retVals := make([][]example.User, len(keys))
	thunks := make(map[int]func() ([]example.User, error), len(keys))
	errors := make([]error, len(keys))

	if l.redisConfig != nil && l.redisConfig.GetManyFunc != nil {
		// using Redis and GetManyFunc is set
		rKeys := make([]string, len(keys))
		for idx, key := range keys {
			rKeys[idx] = UserSliceLoaderCacheKeyPrefix + strconv.FormatInt(int64(key), 10)
		}
		vS, errs, err := l.redisConfig.GetManyFunc(context.Background(), rKeys)
		if err != nil {
			// return errors for all keys
			for i := range errors {
				errors[i] = err
			}
			return retVals, errors
		} else if len(vS) != len(keys) || len(errs) != len(keys) {
			// return errors for all keys, invalid lengths returned
			for i := range errors {
				errors[i] = ErrUserSliceLoaderGetManyLength
			}
		} else {
			l.mu.Lock()
			l.unsafeBatchSet()
			l.mu.Unlock()
			for i, err := range errs {
				if err != nil {
					l.mu.Lock()
					if _, thunk := l.addToBatchUnsafe(keys[i]); thunk != nil {
						thunks[i] = thunk
					}
				} else {
					v := vS[i]
					if v == "" || v == "null" {
						// key found, empty value, return nil
						retVals[i] = nil
					} else {
						var ret []example.User
						if err := l.redisConfig.ObjUnmarshal([]byte(v), &ret); err == nil {
							retVals[i] = nil
						}
					}
				}
			}
		}
	} else {
		// not using Redis or GetManyFunc is not set
		for i, key := range keys {
			if v, thunk := l.LoadThunk(key); thunk != nil {
				thunks[i] = thunk
			} else {
				retVals[i] = v
			}
		}
	}
	for i, thunk := range thunks {
		retVals[i], errors[i] = thunk()
	}

	return retVals, errors
}

// LoadAllThunk returns a function that when called will block waiting for a Users.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
// TODO: Add support for Redis GetManyFunc
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

// redisPrime will set the key value pair in Redis
func (l *UserSliceLoader) redisPrime(key int, value []example.User) bool {
	if err := l.redisConfig.SetFunc(context.Background(), UserSliceLoaderCacheKeyPrefix+strconv.FormatInt(int64(key), 10), value, l.redisConfig.SetTTL); err != nil {
		return false
	} else if l.hookAfterSet != nil {
		l.hookAfterSet(key, value)
	}
	return true
}

// unsafePrime will prime the cache with the given key and value if the key does not exist. This method is not thread safe.
func (l *UserSliceLoader) unsafePrime(key int, value []example.User, forceReplace bool) bool {
	if l.redisConfig != nil {
		// using Redis
		return l.redisPrime(key, value)
	}
	if l.hookExternalCacheSet != nil {
		// make a copy when writing to the cache, its easy to pass a pointer in from a loop var
		// and end up with the whole cache pointing to the same value.
		cpy := make([]example.User, len(value))
		copy(cpy, value)
		if err := l.hookExternalCacheSet(key, cpy); err != nil {
			return false
		}
		if l.hookAfterSet != nil {
			l.hookAfterSet(key, value)
		}
		return true
	}
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
	if l.redisConfig != nil {
		// using Redis
		for i, key := range keys {
			ret[i] = l.redisPrime(key, values[i])
		}
	} else {
		l.mu.Lock()
		for i, key := range keys {
			ret[i] = l.unsafePrime(key, values[i], false)
		}
		l.mu.Unlock()
	}
	return ret
}

// Prime the cache with the provided key and value. If the key already exists, no change is made
// and false is returned.
// (To forcefully prime the cache, clear the key first with loader.clear(key).prime(key, value).)
func (l *UserSliceLoader) Prime(key int, value []example.User) bool {
	if l.redisConfig != nil {
		// using Redis
		b := l.redisPrime(key, value)
		if l.hookAfterPrime != nil {
			l.hookAfterPrime(key, value)
		}
		return b
	} else {
		l.mu.Lock()
		found := l.unsafePrime(key, value, false)
		l.mu.Unlock()
		if l.hookAfterPrime != nil {
			l.hookAfterPrime(key, value)
		}
		return found
	}
}

// ForcePrime the cache with the provided key and value. If the key already exists, value is replaced
// (This removes the requirement to clear the key first with loader.clear(key).prime(key, value))
func (l *UserSliceLoader) ForcePrime(key int, value []example.User) {
	l.batchResultSet(key, value)
	if l.hookAfterPrime != nil {
		l.hookAfterPrime(key, value)
	}
}

// Clear the value at key from the cache, if it exists
func (l *UserSliceLoader) Clear(key int) {
	if l.redisConfig != nil {
		// using Redis
		l.redisConfig.DeleteFunc(context.Background(), UserSliceLoaderCacheKeyPrefix+strconv.FormatInt(int64(key), 10))
		if l.hookAfterClear != nil {
			l.hookAfterClear(key)
		}
		return
	}
	if l.hookExternalCacheDelete != nil {
		l.hookExternalCacheDelete(key)
		if l.hookAfterClear != nil {
			l.hookAfterClear(key)
		}
		return
	}

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

	if l.hookAfterClear != nil {
		l.hookAfterClear(key)
	}
}

// ClearAll clears all values from the cache
func (l *UserSliceLoader) ClearAll() {
	if l.redisConfig != nil {
		// using Redis
		if l.redisConfig.GetKeysFunc != nil {
			// get all keys from Redis
			keys, _ := l.redisConfig.GetKeysFunc(context.Background(), UserSliceLoaderCacheKeyPrefix+"*")
			// delete all these keys from Redis
			if l.redisConfig.DeleteManyFunc != nil {
				l.redisConfig.DeleteManyFunc(context.Background(), keys)
			} else {
				for _, key := range keys {
					l.redisConfig.DeleteFunc(context.Background(), key)
				}
			}
			if l.hookAfterClearAll != nil {
				l.hookAfterClearAll()
			}
		}
		return
	}
	if l.hookExternalCacheClearAll != nil {
		l.hookExternalCacheClearAll()
		if l.hookAfterClearAll != nil {
			l.hookAfterClearAll()
		}
		return
	}

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

	if l.hookAfterClearAll != nil {
		l.hookAfterClearAll()
	}
}

// ClearExpired clears all expired values from the cache if cache expiration is being used
func (l *UserSliceLoader) ClearExpired() {
	if l.redisConfig != nil {
		// using Redis. Nothing to do, TTL will handle this
		return
	}
	if l.expireAfter > 0 {
		// using cache expiration
		tNow := time.Now().UnixNano()
		l.mu.Lock()
		for cacheKey, cacheItem := range l.cacheExpire {
			if cacheItem != nil && tNow > cacheItem.Expires {
				// value has expired
				delete(l.cacheExpire, cacheKey)
				if l.hookAfterExpired != nil {
					l.hookAfterExpired(cacheKey)
				}
			}
		}
		l.mu.Unlock()
	}
}

// unsafeSet will set the key to value without any locks or checks. This method is not thread safe.
func (l *UserSliceLoader) unsafeSet(key int, value []example.User) {
	if l.redisConfig != nil {
		// using Redis
		l.redisPrime(key, value)
		return
	}
	if l.hookExternalCacheSet != nil {
		l.hookExternalCacheSet(key, value)
		if l.hookAfterSet != nil {
			l.hookAfterSet(key, value)
		}
		return
	}

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

	if l.hookAfterSet != nil {
		l.hookAfterSet(key, value)
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
	if l.hookBeforeFetch != nil {
		l.hookBeforeFetch(b.keys, "UserSliceLoader")
	}
	b.data, b.errors = l.fetch(b.keys)
	if l.hookAfterFetch != nil {
		l.hookAfterFetch(b.keys, "UserSliceLoader")
	}
	close(b.done)
}

// MarshalUserSliceLoaderToString is a helper method to marshal a UserSliceLoader to a string
func (l *UserSliceLoader) MarshalUserSliceLoaderToString(v int) string {
	ret, _ := l.redisConfig.ObjMarshal(v)
	return string(ret)
}
