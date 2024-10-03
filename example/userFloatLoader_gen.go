// Code generated by github.com/viocle/dataloaden, DO NOT EDIT.

package example

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"
)

const (
	UserFloatLoaderCacheKeyPrefix = "{DataLoaderUserFloatLoader}:"
)

var (
	ErrUserFloatLoaderGetManyLength = errors.New("redis error, invalid length returned from GetManyFunc")
)

// UserFloatLoaderConfig captures the config to create a new UserFloatLoader
type UserFloatLoaderConfig struct {
	// Fetch is a method that provides the data for the loader
	Fetch func(keys []float64) ([]*User, []error)

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
	HookExternalCacheGet func(key float64) (*User, bool)

	// HookExternalCacheSet is a method that provides the ability to set a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	HookExternalCacheSet func(key float64, value *User) error

	// HookBeforeFetch is a method that provides the ability to delete/clear a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	HookExternalCacheDelete func(key float64) error

	// HookExternalCacheClearAll is a method that provides the ability to clear all keys in an external cache with an external hook.
	HookExternalCacheClearAll func() error

	// HookBeforeFetch is called right before a fetch is performed
	HookBeforeFetch func(keys []float64, loaderName string)

	// HookAfterFetch is called right after a fetch is performed
	HookAfterFetch func(keys []float64, loaderName string)

	// HookAfterSet is called after a value is set in the cache
	HookAfterSet func(key float64, value *User)

	// HookAfterPrime is called after a value is primed in the cache using Prime, ForcePrime, or PrimeMany.
	HookAfterPrime func(key float64, value *User)

	// HookAfterPrimeMany is called after values are primed in the cache using PrimeMany. If not set then HookAfterPrime will be used if set.
	HookAfterPrimeMany func(keys []float64, values []*User)

	// HookAfterClear is called after a value is cleared from the cache
	HookAfterClear func(key float64)

	// HookAfterClearAll is called after all values are cleared from the cache
	HookAfterClearAll func()

	// HookAfterClearAllPrefix is called after all values are cleared from the cache
	HookAfterClearAllPrefix func(prefix string)

	// HookAfterExpired is called after a value is cleared in the cache due to expiration
	HookAfterExpired func(key float64)

	// RedisConfig is used to configure a UserFloatLoader backed by Redis, disabling the internal cache.
	RedisConfig *UserFloatLoaderRedisConfig
}

// UserFloatLoaderCacheItem defines a cache item when using dataloader cache expiration where expireAfter > 0
type UserFloatLoaderCacheItem struct {
	// Expires contains the time this CacheItem expires
	Expires int64

	// Value contains the cached *User
	Value *User
}

// expired returns true if the cache item has expired
func (c *UserFloatLoaderCacheItem) expired(now int64) bool {
	return c.Expires < now
}

// NewUserFloatLoader creates a new UserFloatLoader given a fetch, wait, and maxBatch
func NewUserFloatLoader(config UserFloatLoaderConfig) *UserFloatLoader {
	l := &UserFloatLoader{
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
		hookAfterPrimeMany:        config.HookAfterPrimeMany,
		hookAfterClear:            config.HookAfterClear,
		hookAfterClearAll:         config.HookAfterClearAll,
		hookAfterClearAllPrefix:   config.HookAfterClearAllPrefix,
		hookAfterExpired:          config.HookAfterExpired,
		redisConfig:               config.RedisConfig,
	}
	if config.RedisConfig != nil {
		// validate we have all the required Redis functions. If not, force disable Redis
		if l.redisConfig.GetFunc != nil && l.redisConfig.SetFunc != nil && l.redisConfig.DeleteFunc != nil {
			// all required Redis functions are present, enable Redis
			l.redisConfig = &UserFloatLoaderRedisConfig{
				SetTTL:          config.RedisConfig.SetTTL,          // optional
				GetFunc:         config.RedisConfig.GetFunc,         // (GET)
				GetManyFunc:     config.RedisConfig.GetManyFunc,     // (MGET) optional, but recommended for LoadAll performance
				SetFunc:         config.RedisConfig.SetFunc,         // (SET)
				SetManyFunc:     config.RedisConfig.SetManyFunc,     // (SET/MSET) optional, but recommended for PrimeMany performance. Suggested to use pipeline or MSET
				DeleteFunc:      config.RedisConfig.DeleteFunc,      // (DEL)
				DeleteManyFunc:  config.RedisConfig.DeleteManyFunc,  // (DEL) optional, but recommended for ClearAll performance
				GetKeysFunc:     config.RedisConfig.GetKeysFunc,     // optional, but recommended for ClearAll support
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
			l.batchResultSet = func(key float64, value *User) {
				l.redisConfig.SetFunc(context.Background(), UserFloatLoaderCacheKeyPrefix+strconv.FormatFloat(key, 'f', -1, 64), value, l.redisConfig.SetTTL)
			}
			if l.redisConfig.KeyToStringFunc == nil {
				l.redisConfig.KeyToStringFunc = l.MarshalUserFloatLoaderToString
			}
		}
	}
	if l.redisConfig == nil {
		// set the default batchResultSet
		l.batchResultSet = func(key float64, value *User) {
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
	return l
}

// UserFloatLoaderRedisConfig is used to configure a UserFloatLoader backed by Redis. GetFunc, SetFunc, and DeleteFunc are required if using Redis. If any function is not provided, Redis will be disabled and internal caching will be used.
type UserFloatLoaderRedisConfig struct {
	// SetTTL is the TTL (Time To Live) for a key to live in Redis on set. If nil, no TTL will be set.
	SetTTL *time.Duration

	// GetFunc should get a value from Redis given a key and return the raw string value.
	GetFunc func(ctx context.Context, key string) (string, error)

	// GetManyFunc should get one or more values from Redis given a set of keys and return the raw string values, errors the size of keys with non nil values for keys not found, and an error if any other error occurred running the command
	// If not set then GetFunc will be used instead, but will be called one at a time for each key
	GetManyFunc func(ctx context.Context, keys []string) ([]string, []error, error)

	// SetFunc should set a value in Redis given a key and value with an optional ttl (Time To Live)
	SetFunc func(ctx context.Context, key string, value interface{}, ttl *time.Duration) error

	// SetManyFunc should set one or more values in Redis given a set of keys and values with an optional ttl (Time To Live)
	// If not set then SetFunc will be used instead, but will be called one at a time for each key. To implement, look at using a pipeline or MSET
	SetManyFunc func(ctx context.Context, keys []string, values []interface{}, ttl *time.Duration) ([]error, error)

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

	// HookAfterObjUnmarshal is a method that provides the ability to run a function after an object is unmarshaled. This is useful for setting up any object state after unmarshaling or logging.
	HookAfterObjUnmarshal func(*User) *User

	// KeyToStringFunc provides you the ability to specify your own function to convert a key to a string, which will be used instead of serialization.
	// This is only used for non standard types that need to be serialized. If not set, the ObjMarshal function (user defined or default) will be used to serialize a key into a string value
	// Example: If you have a struct with a String() function that returns a string representation of the struct, you can set this function to that function.
	//
	// type MyStruct struct {
	//     ID string
	//     OrgID string
	// }
	// ...
	// UserFloatLoaderRedisConfig{
	//		KeyToStringFunc = func(key float64) string { return m.ID + ":" + m.OrgID }
	// }
	// ...
	// Or if your key type has a String() function that returns a string representation of the key, you can set this function like this:
	// UserFloatLoaderRedisConfig{
	//		KeyToStringFunc = func(key float64) string { return key.String() }
	// }
	KeyToStringFunc func(key float64) string
}

// UserFloatLoader batches and caches requests
type UserFloatLoader struct {
	// this method provides the data for the loader
	fetch func(keys []float64) ([]*User, []error)

	// optional Redis configuration
	redisConfig *UserFloatLoaderRedisConfig

	// lazily created cache

	cacheExpire map[float64]*UserFloatLoaderCacheItem

	cache map[float64]*User

	// the current batch. keys will continue to be collected until timeout is hit,
	// then everything will be sent to the fetch method and out to the listeners
	batch *userFloatLoaderBatch

	// batchResultSet sets the batch result
	batchResultSet func(float64, *User)

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
	hookExternalCacheGet func(key float64) (*User, bool)

	// hookExternalCacheSet is a method that provides the ability to set a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	hookExternalCacheSet func(key float64, value *User) error

	// hookBeforeFetch is a method that provides the ability to delete/clear a key in an external cache with an external hook.
	// This replaces the use of the internal cache.
	hookExternalCacheDelete func(key float64) error

	// hookExternalCacheClearAll is a method that provides the ability to clear all keys in an external cache with an external hook.
	hookExternalCacheClearAll func() error

	// hookBeforeFetch is called right before a fetch is performed
	hookBeforeFetch func(keys []float64, loaderName string)

	// hookAfterFetch is called right after a fetch is performed
	hookAfterFetch func(keys []float64, loaderName string)

	// hookAfterSet is called after a value is set in the cache
	hookAfterSet func(key float64, value *User)

	// hookAfterPrime is called after a value is primed in the cache using Prime, ForcePrime, or PrimeMany
	hookAfterPrime func(key float64, value *User)

	// hookAfterPrimeMany is called after values are primed in the cache using PrimeMany. If not set then hookAfterPrime will be used if set.
	hookAfterPrimeMany func(keys []float64, values []*User)

	// hookAfterClear is called after a value is cleared from the cache
	hookAfterClear func(key float64)

	// hookAfterClearAll is called after all values are cleared from the cache
	hookAfterClearAll func()

	// HookAfterClearAllPrefix is called after all values are cleared from the cache
	hookAfterClearAllPrefix func(prefix string)

	// hookAfterExpired is called after a value is cleared in the cache due to expiration
	hookAfterExpired func(key float64)

	// pool of batches
	batchPool sync.Pool
}

type userFloatLoaderBatch struct {
	loader    *UserFloatLoader
	now       int64
	done      chan struct{}
	keysMap   map[float64]int
	keys      []float64
	data      []*User
	errors    []error
	closing   bool
	lock      sync.Mutex
	reqCount  int
	checkedIn int
}

// Load a User by key, batching and caching will be applied automatically
func (l *UserFloatLoader) Load(key float64) (*User, error) {
	v, f := l.LoadThunk(key)
	if f != nil {
		return f()
	}
	return v, nil
}

// unsafeBatchSet creates a new batch if one does not exist, otherwise it will reuse the existing batch
func (l *UserFloatLoader) unsafeBatchSet() {
	if l.batch == nil {
		b := l.batchPool.Get().(*userFloatLoaderBatch)
		// create new batch re-using our keysMap and keys fields
		l.batch = &userFloatLoaderBatch{loader: l, now: 0, done: make(chan struct{}), keysMap: b.keysMap, keys: b.keys[:0], data: nil, errors: nil, reqCount: 0, checkedIn: 0, lock: sync.Mutex{}}
	} else if l.batch.now == 0 {
		// have a batch but first use, set the start time
		l.batch.now = time.Now().UnixNano()
	}
}

// createNewBatch creates a new batch
func (l *UserFloatLoader) createNewBatch() *userFloatLoaderBatch {
	return &userFloatLoaderBatch{
		loader:    l,
		now:       0,
		done:      make(chan struct{}),
		keysMap:   make(map[float64]int, l.maxBatch),
		keys:      make([]float64, 0, l.maxBatch),
		data:      nil,
		errors:    nil,
		lock:      sync.Mutex{},
		reqCount:  0,
		checkedIn: 0,
	}
}

// LoadThunk returns a function that when called will block waiting for a User.
// This method should be used if you want one goroutine to make requests to many
// different data loaders without blocking until the thunk is called.
func (l *UserFloatLoader) LoadThunk(key float64) (*User, func() (*User, error)) {
	if l.redisConfig != nil {
		// using Redis
		v, err := l.redisConfig.GetFunc(context.Background(), UserFloatLoaderCacheKeyPrefix+strconv.FormatFloat(key, 'f', -1, 64))
		if err == nil {
			// found in Redis, attempt to return value
			if v == "" || v == "null" {
				// key found, empty value, return nil
				return nil, nil
			}
			ret := &User{}
			if err := l.redisConfig.ObjUnmarshal([]byte(v), ret); err == nil {
				if l.redisConfig.HookAfterObjUnmarshal != nil {
					ret = l.redisConfig.HookAfterObjUnmarshal(ret)
				}
				return ret, nil
			}
			// error unmarshalling, just add to batch
		}
		// not found in Redis or error, continue
		l.mu.Lock() // unsafeAddToBatch will unlock
	} else {
		if l.hookExternalCacheGet != nil {
			if v, ok := l.hookExternalCacheGet(key); ok {
				return v, nil
			}
			// not found in external cache, continue
			l.mu.Lock() // unsafeAddToBatch will unlock
		} else {
			l.mu.Lock() // unsafeAddToBatch will unlock

			if l.expireAfter <= 0 && len(l.cache) > 0 {
				// not using cache expiration
				if it, ok := l.cache[key]; ok {
					l.mu.Unlock()
					return it, nil
				}
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
			}

		}
	}
	return l.unsafeAddToBatch(key)
}

// unsafeAddToBatch adds the key to the current batch and returns a thunk to be called later. This method is not thread safe. Expects l.mu.lock() to have been called prior to calling this method.
func (l *UserFloatLoader) unsafeAddToBatch(key float64) (*User, func() (*User, error)) {
	l.unsafeBatchSet()
	batch := l.batch
	pos := batch.keyIndex(l, key)
	l.mu.Unlock()

	return nil, func() (*User, error) {
		<-batch.done

		// batch has been closed, pull result
		data, err := batch.getResult(pos)

		if err == nil && l.redisConfig == nil {
			// not using Redis, set the cache here, otherwise it'll be done on batch fetch completion
			l.batchResultSet(key, data)
		}

		return data, err
	}
}

// LoadAll fetches many keys at once. It will be broken into appropriate sized
// sub batches depending on how the loader is configured
func (l *UserFloatLoader) LoadAll(keys []float64) ([]*User, []error) {
	if len(keys) == 0 {
		return nil, nil
	}
	retVals := make([]*User, len(keys))
	thunks := make(map[int]func() (*User, error), len(keys))
	errors := make([]error, len(keys))

	if l.redisConfig != nil && l.redisConfig.GetManyFunc != nil {
		// using Redis and GetManyFunc is set
		rKeys := make([]string, len(keys))
		for idx, key := range keys {
			rKeys[idx] = UserFloatLoaderCacheKeyPrefix + strconv.FormatFloat(key, 'f', -1, 64)
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
				errors[i] = ErrUserFloatLoaderGetManyLength
			}
		} else {
			for i, err := range errs {
				if err != nil {
					l.mu.Lock() // unsafeAddToBatch will unlock
					if _, thunk := l.unsafeAddToBatch(keys[i]); thunk != nil {
						thunks[i] = thunk
					}
				} else {
					if vS[i] == "" || vS[i] == "null" {
						// key found, empty value, return nil
						retVals[i] = nil
					} else {
						ret := &User{}
						if err := l.redisConfig.ObjUnmarshal([]byte(vS[i]), ret); err == nil {
							if l.redisConfig.HookAfterObjUnmarshal != nil {
								ret = l.redisConfig.HookAfterObjUnmarshal(ret)
							}
							retVals[i] = ret
						} else {
							l.mu.Lock() // unsafeAddToBatch will unlock
							if _, thunk := l.unsafeAddToBatch(keys[i]); thunk != nil {
								thunks[i] = thunk
							}
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
func (l *UserFloatLoader) LoadAllThunk(keys []float64) func() ([]*User, []error) {
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

// redisPrime will set the key value pair in Redis
func (l *UserFloatLoader) redisPrime(key float64, value *User) bool {
	if err := l.redisConfig.SetFunc(context.Background(), UserFloatLoaderCacheKeyPrefix+strconv.FormatFloat(key, 'f', -1, 64), value, l.redisConfig.SetTTL); err != nil {
		return false
	} else if l.hookAfterSet != nil {
		l.hookAfterSet(key, value)
	}
	return true
}

// unsafePrime will prime the cache with the given key and value if the key does not exist. This method is not thread safe.
func (l *UserFloatLoader) unsafePrime(key float64, value *User, forceReplace bool) bool {
	if l.redisConfig != nil {
		// using Redis
		return l.redisPrime(key, value)
	}
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

	if l.expireAfter <= 0 {
		// not using cache expiration

		if _, found = l.cache[key]; found && forceReplace {
			delete(l.cache, key)
		}
		if !found || forceReplace {
			// make a copy when writing to the cache, its easy to pass a pointer in from a loop var
			// and end up with the whole cache pointing to the same value.
			cpy := *value
			l.unsafeSet(key, &cpy)
		}

	} else {
		// using cache expiration
		if _, found = l.cacheExpire[key]; found && forceReplace {
			delete(l.cacheExpire, key)
		}
		if !found || forceReplace {
			// make a copy when writing to the cache, its easy to pass a pointer in from a loop var
			// and end up with the whole cache pointing to the same value.
			cpy := *value
			l.unsafeSet(key, &cpy)
		}
	}

	return !found || forceReplace
}

// PrimeManyNoReturn will prime the cache with the given keys and values. Value index is matched to key index. Wraps the PrimeMany and ignores the return values. Helpful if you want to connect up to HookAfterPrimeMany in another dataloader
func (l *UserFloatLoader) PrimeManyNoReturn(keys []float64, values []*User) {
	l.PrimeMany(keys, values)
}

// PrimeMany will prime the cache with the given keys and values. Value index is matched to key index.
func (l *UserFloatLoader) PrimeMany(keys []float64, values []*User) []bool {
	if len(keys) != len(values) {
		// keys and values must be the same length
		return make([]bool, len(keys))
	}
	ret := make([]bool, len(keys))
	var hookKeys []float64
	var hookValues []*User
	if l.hookAfterPrimeMany != nil {
		hookKeys = make([]float64, 0, len(keys))
		hookValues = make([]*User, 0, len(values))
	}
	if l.redisConfig != nil {
		// using Redis
		if l.redisConfig.SetManyFunc != nil && len(keys) > 1 {
			// SetManyFunc is set and items to prime is >1
			// convert values slice (of *User) to interface slice
			vSet := make([]interface{}, len(values))
			for i := range values {
				vSet[i] = values[i]
			}
			// convert keys slice (of float64) to string slice
			kSet := make([]string, len(keys))
			for i, key := range keys {
				kSet[i] = UserFloatLoaderCacheKeyPrefix + strconv.FormatFloat(key, 'f', -1, 64)
			}
			// call SetManyFunc with our keys and values
			retErr, err := l.redisConfig.SetManyFunc(context.Background(), kSet, vSet, l.redisConfig.SetTTL)
			if err == nil {
				// set the return values based on each key's error
				for i, err := range retErr {
					ret[i] = err == nil
					if ret[i] {
						// success, call hookAfterSet, hookAfterPrime, and prepare for hookAfterPrimeMany if any are set
						if l.hookAfterSet != nil {
							l.hookAfterSet(keys[i], values[i])
						}
						if l.hookAfterPrimeMany != nil {
							hookKeys = append(hookKeys, keys[i])
							hookValues = append(hookValues, values[i])
						} else if l.hookAfterPrime != nil {
							l.hookAfterPrime(keys[i], values[i])
						}
					}
				}
				// call hookAfterPrimeMany if set
				if l.hookAfterPrimeMany != nil {
					l.hookAfterPrimeMany(hookKeys, hookValues)
				}
			}
		} else {
			// fallback to using redisPrime (one at a time)
			for i, key := range keys {
				ret[i] = l.redisPrime(key, values[i])
				if ret[i] {
					// success, call hookAfterPrime and prepare for hookAfterPrimeMany if any are set. redisPrime will handle the call to hookAfterSet
					if l.hookAfterPrimeMany != nil {
						hookKeys = append(hookKeys, keys[i])
						hookValues = append(hookValues, values[i])
					} else if l.hookAfterPrime != nil {
						l.hookAfterPrime(keys[i], values[i])
					}
				}
			}
			// call hookAfterPrimeMany if set
			if l.hookAfterPrimeMany != nil {
				l.hookAfterPrimeMany(hookKeys, hookValues)
			}
		}
	} else {
		l.mu.Lock()
		for i, key := range keys {
			ret[i] = l.unsafePrime(key, values[i], false)
			if ret[i] {
				// success, call hookAfterPrime and prepare for hookAfterPrimeMany if any are set. unsafePrime will handle the call to hookAfterSet
				if l.hookAfterPrimeMany != nil {
					hookKeys = append(hookKeys, keys[i])
					hookValues = append(hookValues, values[i])
				} else if l.hookAfterPrime != nil {
					l.hookAfterPrime(keys[i], values[i])
				}
			}
		}
		l.mu.Unlock()
		// call hookAfterPrimeMany if set
		if l.hookAfterPrimeMany != nil {
			l.hookAfterPrimeMany(hookKeys, hookValues)
		}
	}
	return ret
}

// Prime the cache with the provided key and value. If the key already exists, no change is made
// and false is returned.
// (To forcefully prime the cache, clear the key first with loader.clear(key).prime(key, value).)
func (l *UserFloatLoader) Prime(key float64, value *User) bool {
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
func (l *UserFloatLoader) ForcePrime(key float64, value *User) {
	l.batchResultSet(key, value)
	if l.hookAfterPrime != nil {
		l.hookAfterPrime(key, value)
	}
}

// Clear the value at key from the cache, if it exists
func (l *UserFloatLoader) Clear(key float64) {
	if l.redisConfig != nil {
		// using Redis
		l.redisConfig.DeleteFunc(context.Background(), UserFloatLoaderCacheKeyPrefix+strconv.FormatFloat(key, 'f', -1, 64))
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

// ClearAllPrefix clears all values from the cache that match the given prefix (after the cache key prefix if using Redis) Prefix filtering is only used when using Redis and GetKeysFunc is defined or your key type is a string, otherwise all keys are cleared.
func (l *UserFloatLoader) ClearAllPrefix(prefix string) {
	if l.redisConfig != nil {
		// using Redis
		if l.redisConfig.GetKeysFunc != nil {
			// get all keys from Redis
			keys, _ := l.redisConfig.GetKeysFunc(context.Background(), UserFloatLoaderCacheKeyPrefix+prefix+"*")
			// delete all these keys from Redis
			if l.redisConfig.DeleteManyFunc != nil {
				l.redisConfig.DeleteManyFunc(context.Background(), keys)
			} else {
				for _, key := range keys {
					l.redisConfig.DeleteFunc(context.Background(), key)
				}
			}
			if l.hookAfterClearAllPrefix != nil {
				l.hookAfterClearAllPrefix(prefix)
			}
			if l.hookAfterClearAll != nil {
				l.hookAfterClearAll()
			}
		}
		return
	}
	if l.hookExternalCacheClearAll != nil {
		l.hookExternalCacheClearAll()
		if l.hookAfterClearAllPrefix != nil {
			l.hookAfterClearAllPrefix(prefix)
		}
		if l.hookAfterClearAll != nil {
			l.hookAfterClearAll()
		}
		return
	}

	if l.expireAfter <= 0 {
		// not using cache expiration

		l.mu.Lock()

		l.cache = make(map[float64]*User, l.maxBatch)

		l.mu.Unlock()

	} else {
		// using cache expiration
		l.mu.Lock()

		l.cacheExpire = make(map[float64]*UserFloatLoaderCacheItem, l.maxBatch)

		l.mu.Unlock()
	}

	if l.hookAfterClearAllPrefix != nil {
		l.hookAfterClearAllPrefix(prefix)
	}
	if l.hookAfterClearAll != nil {
		l.hookAfterClearAll()
	}
}

// ClearAll clears all values from the cache
func (l *UserFloatLoader) ClearAll() {
	l.ClearAllPrefix("")
}

// ClearExpired clears all expired values from the cache if cache expiration is being used
func (l *UserFloatLoader) ClearExpired() {
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
func (l *UserFloatLoader) unsafeSet(key float64, value *User) {
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
			l.cache = make(map[float64]*User, l.maxBatch)
		}
		l.cache[key] = value

	} else {
		// using cache expiration
		if l.cacheExpire == nil {
			l.cacheExpire = make(map[float64]*UserFloatLoaderCacheItem, l.maxBatch)
		}
		l.cacheExpire[key] = &UserFloatLoaderCacheItem{Expires: time.Now().UnixNano() + l.expireAfter, Value: value}
	}

	if l.hookAfterSet != nil {
		l.hookAfterSet(key, value)
	}
}

// keyIndex will return the location of the key in the batch, if its not found
// it will add the key to the batch
func (b *userFloatLoaderBatch) keyIndex(l *UserFloatLoader, key float64) int {
	b.reqCount++
	if i, ok := b.keysMap[key]; ok {
		return i
	}

	pos := len(b.keysMap)
	b.keysMap[key] = pos
	b.keys = append(b.keys, key)
	if pos == 0 {
		go b.startTimer(l)
	}

	// have we reached out max batch size?
	if l.maxBatch != 0 && pos >= l.maxBatch-1 {
		if !b.closing {
			// not already closing, close the batch and call end
			b.closing = true
			l.batch = nil
			go b.end(l)
		}
	}

	return pos
}

// startTimer will wait the desired wait time before sending the batch unless another batch limit had been reached
func (b *userFloatLoaderBatch) startTimer(l *UserFloatLoader) {
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
func (b *userFloatLoaderBatch) end(l *UserFloatLoader) {
	if l.hookBeforeFetch != nil {
		l.hookBeforeFetch(b.keys, "UserFloatLoader")
	}
	b.data, b.errors = l.fetch(b.keys)
	if l.redisConfig != nil && len(b.errors) > 0 {
		// using Redis, set the cache here for all results without an error
		if len(b.errors) > 1 && l.redisConfig.SetManyFunc != nil {
			// multiple keys, build key/value set of non errors
			kSet := make([]string, 0, len(b.keys))
			vSet := make([]interface{}, 0, len(b.keys))
			for i, key := range b.keys {
				if b.errors[i] == nil {
					// convert keys slice (of float64) to string slice
					kSet = append(kSet, UserFloatLoaderCacheKeyPrefix+strconv.FormatFloat(key, 'f', -1, 64))
					vSet = append(vSet, b.data[i])
				}
			}
			if len(kSet) > 0 {
				// call SetManyFunc with our keys and values
				l.redisConfig.SetManyFunc(context.Background(), kSet, vSet, l.redisConfig.SetTTL)
			}
		} else {
			// only one key or SetManyFunc not set, set the value(s) if no error using batchResultSet
			for i, key := range b.keys {
				if b.errors[i] == nil {
					l.batchResultSet(key, b.data[i])
				}
			}
		}
	}
	if l.hookAfterFetch != nil {
		l.hookAfterFetch(b.keys, "UserFloatLoader")
	}
	// close done channel to signal all thunks to unblock
	close(b.done)
}

// getResult will return the result for the given position from the batch
func (b *userFloatLoaderBatch) getResult(pos int) (*User, error) {
	var data *User
	b.lock.Lock()
	if pos < len(b.data) {
		data = b.data[pos]
	}

	var err error
	// its convenient to be able to return a single error for everything
	if len(b.errors) == 1 {
		err = b.errors[0]
	} else if b.errors != nil {
		err = b.errors[pos]
	}

	// check if all thunks have checked in and if so, return batch to pool
	b.checkedIn++
	if b.checkedIn >= b.reqCount {
		// reset
		b.reqCount = 0
		b.checkedIn = 0
		clear(b.keysMap)
		clear(b.keys)
		b.lock.Unlock()
		// all thunks have checked in, return batch to pool for re-use
		b.loader.batchPool.Put(b)
	} else {
		b.lock.Unlock()
	}

	// return data and error
	return data, err
}

// MarshalUserFloatLoaderToString is a helper method to marshal a UserFloatLoader to a string
func (l *UserFloatLoader) MarshalUserFloatLoaderToString(v float64) string {
	ret, _ := l.redisConfig.ObjMarshal(v)
	return string(ret)
}
