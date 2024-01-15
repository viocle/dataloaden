### The DATALOADer gENerator [![Go Report Card](https://goreportcard.com/badge/github.com/viocle/dataloaden)](https://goreportcard.com/report/github.com/viocle/dataloaden)

This fork Requires golang 1.21+

This is a tool for generating type safe data loaders for go, inspired by https://github.com/facebook/dataloader.

The intended use is in graphql servers, to reduce the number of queries being sent to the database. These dataloader
objects should be request scoped and short lived. They should be cheap to create in every request even if they dont
get used. If desired, these are options to maintain a dataloader for re-use/sharing accross multiple requests with cache expiration.

#### Getting started

From inside the package you want to have the dataloader in:
```bash
go run github.com/viocle/dataloaden UserLoader string *github.com/dataloaden/example.User
```

This will generate a dataloader called `UserLoader` that looks up `*github.com/dataloaden/example.User`'s objects 
based on a `string` key. 

You also have an optional param of type bool you can pass after your type to disable cache expiration if desired. Default value is false if not provided.
```bash
go run github.com/viocle/dataloaden UserLoader string *github.com/dataloaden/example.User true
```

In another file in the same package, create the constructor method:
```go
func NewLoader() *UserLoader {
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
```

Then wherever you want to call the dataloader
```go
loader := NewLoader()

user, err := loader.Load("123")
```

This method will block for a short amount of time, waiting for any other similar requests to come in, call your fetch
function once. It also caches values and wont request duplicates in a batch.

You need to use the built in New{{Loader}} function. This ensures the sync.Pool, initial batch, and optional expiration are all initialized properly. Example:
```go
loader := NewUserLoader(UserLoaderConfig{
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
```
#### Returning Slices

You may want to generate a dataloader that returns slices instead of single values. Both key and value types can be a 
simple go type expression: 

```bash
go run github.com/viocle/dataloaden UserSliceLoader string []*github.com/dataloaden/example.User
```

Now each key is expected to return a slice of values and the `fetch` function has the return type `[][]*User`.

#### Using with go modules

Create a tools.go that looks like this:
```go
// +build tools

package main

import _ "github.com/viocle/dataloaden"
```

This will allow go modules to see the dependency.

You can invoke it from anywhere within your module now using `go run github.com/viocle/dataloaden` and 
always get the pinned version.

You can also build out your own tool.go file to expand on the configuration of the dataloader. See example/custom_tool/tool.go for more information.

#### Wait, how do I use context with this?

I don't think context makes sense to be passed through a data loader. Consider a few scenarios:
1. a dataloader shared between requests: request A and B both get batched together, which context should be passed to the DB? context.Background is probably more suitable.
2. a dataloader per request for graphql: two different nodes in the graph get batched together, they have different context for tracing purposes, which should be passed to the db? neither, you should just use the root request context.


So be explicit about your context:
```go
func NewLoader(ctx context.Context) *UserLoader {
	return NewUserLoader(UserLoaderConfig{
		Wait:     2 * time.Millisecond,
		MaxBatch: 100,
		Fetch: func(keys []string) ([]*User, []error) {
			// you now have a ctx to work with
		},
	})
}
```

If you feel like I'm wrong please raise an issue.

#### What's different in this fork?

##### Key differences:
This fork does have some breaking changes. Primary differences are:
1. You must use the New{{Loader}} function with Config to create your dataloaders. Example: NewUserLoder(UserLoaderConfig{...})
2. `Cache Expiration` when you want a long lived dataloader or one that is re-used across multiple requests but do want values to live forever.
3. `Performance Improvements` around memory allocation, key lookup time in large batches, and removal of some unnecessary function wraping around cache hits.
4. `Hooks` allow you to call a function after certain events like when a value is set, deleted, or expired in the cache.
5. External cache `Hooks`. These hooks, HookExternalCacheGet, HookExternalCacheSet, HookExternalCacheDelete, and HookExternalCacheClearAll completely bypass the internal cache allowing you to define your own functions to get/set/delete/clear all key/values from your external source.

##### All changes:
1. Added cache expiration support. When creating a new loader, set the expireAfter time.Duration to the amount of time you want the cached items to be valid for. Cache expiration does not automatically remove the value from the loader's cache but will perform a new fetch if the value is expired when loading a key. Example usage for this would be to create your dataloader once and re-use it for all requests, not creating new ones for each request. This does mean you need to ensure you're properly clearing this dataloader when changes to the object occur so values in the dataloader do not become stale. Cache expiration can be completely removed from your generated code if you specify true in your generate call after your return type definition.
```go
func main() {
	myUserLoader := NewUserLoader(UserLoaderConfig{
			ExpireAfter: 30 * time.Minute, // each cached item will expire 30 minutes after being added
			Wait:     2 * time.Millisecond,
			MaxBatch: 100,
			Fetch: func(keys []string) ([]*User, []error) {
				// ...
			},
		})
	
	// regularly clear expired cache (if desired). See example below.
	go clearDataLoaderExpiredCache()

	// ... some long running process that uses myUserLoader
}
```

To clean up expired cache items on a regular basis, creating the following function and calling it, `go clearDataLoaderExpiredCache()`, after you create your dataloader can be done. Now, every 1 minute, your dataloader(s) will clear expired cached items to free up memory.
```go
func clearDataLoaderExpiredCache() {
	clearNow := time.NewTicker(60 * time.Minute)
	defer clearNow.Stop()
	for {
		// wait ticker
		<-clearNow.C

		// call ClearExpired on all "global" data loaders
		myUserLoader.ClearExpired()
	}
}
```

2. Added ClearAll() which allows you to clear all cached items in loader
3. Generated files will be in camelCase
4. Added GenerateWithPrefix() which allows you to specify the prefix of a generated file
5. Added ClearExpired() which allow you to clear all expired cached items in loader
6. Added ForcePrime(key, value) which allows you to prime the cache with the provided key and value just like Prime() except that if the key already exists in the cache, the value is replaced. This removes the need to call Clear(key).Prime(key, value) if desired
7. Added PrimeMany([]keys, []values) which allows you to prime multiple key/values into the cache with a single call
8. Add configuration to New{{LoadName}}Loader function.
9. Batches are pre-allocated and re-used to lower allocations between Loads.
10. Batches now use a slice as well as a map for key store and lookup. There is a slight memory usage penalty for the duplicate key values in the map at the extra performance increase seen in lookups instead of iterating over the keys in the slice. You can see this new usage in the keyIndex function. This performance change is really apparent when you have large maxBatch values.
11. LoadThunk has the option to now return the value directly instead of having to wrap the cache find in a function to be called. This change was implemented in LoadThunk and return checking is done in Load. If no function was returned then the returned value is to be used.
12. You can define Hook functions to be called after a key is set, cleared, when all keys are cleared, or an item is cleared because it has expired. 
a.`HookAfterSet`: When a key is set in the dataloader, this function will be called if defined. This is performed inside `unsafeSet` so if the key already exists in the dataloader and forceReplace is not true when calling `unsafePrime`, then `HookAfterSet` will not get set.
b.`HookAfterClear`: When a key is cleared in the dataloader, this function will be called if defined. This does not occur when an existing key is being replaced or it's deleted because of expiration.
c.`HookAfterClearAll`: When you call the `ClearAll` function to clear the entire dataloader cache, this function will be called if defined.
d.`HookAfterExpired`: When a key is cleared because it has expired, this function will be called if defined. Clearing expired cached items only occurs when the key is interacted with while being loaded or cleared via `ClearExpired`. To be clear, this is not Hooked at the point the cached item becomes no longer valid, but when it's accessed and is determined as expired.
e. `HookBeforeFetch`: Called right before a fetch is performed. Primarily used for tracing.
f. `HookAfterFetch`: Called right after a fetch is performed. Primarily used for tracing.
g. `HookExternalCacheGet`, `HookExternalCacheSet`, `HookExternalCacheDelete`, and `HookExternalCacheClearAll`: Bypass the internal cache and allows you to define your own functions to work with another cache source like Redis or another shared resource.
13. Redis support. See Redis Support section below for more details.

#### Redis Support

You can use Redis as the cache storage for your dataloader by configuring the RedisConfig value when creating a new instance of a dataloader. Below are the configuration values available and how they're used.

1. `SetTTL` (Optional) Is the KEEPTTL value used in the SET command. This value is passed back to your SetFunc value when being called.
2. `GetFunc` (Required) Is your function to perform a `GET` command.
3. `GetManyFunc` (Optional) Is your function to perform a `MGET` command wich can retrieve multiple keys at once. If this is not set then `GetFunc` will be used instead but will be slower.
4. `SetFunc` (Required) Is your function to perform a `SET` command.
5. `DeleteFunc` (Required) Is your function to perform a `DEL` command.
6. `DeleteManyFunc` (Optional) Is your function to perform a `DEL` command but handle more than one key.
7. `GetKeysFunc` (Optional) Is your function to perform a `KEYS` command with a filter pattern. Required if you want to use the `ClearAll` feature on your dataloader when using Redis.
7. `ObjMarshal` (Optional) You can define your own serializer. If not defined then this gets set to json.Marshal.
8. `ObjUnmarshal` (Optional) You can define your own deserializer. If not defined then this gets set to json.Unmarshal.
9. `KeyToStringFunc` (Optional) Is used to convert your key to a string representation. If this is not set and your key type is a struct, map, and array of non string types, then the default function `Marshal{{.Name}}ToString` will be used to serialize your key into a single value which can be slower than alternatives. If you have a String() method on your type, this is where you should use it.

Example setup:
```
redisClient := SetupRedis() // example uses redis/go-redis package
ttl := 5 * time.Minute
// use json-iterator/go package for optimized json serialization. Confirm compatibility with your types.
JSONItGraphObjects := jsoniter.Config{
		EscapeHTML:                    true,  // Standard library compatible
		ValidateJsonRawMessage:        false, // Standard library compatible
		SortMapKeys:                   false, // dont sort map keys when marshalling
		ObjectFieldMustBeSimpleString: true,  // do not unescape object field. Ex. Field name "Ag\u0065" will not be unescaped to "Age". Only set to true if your objects do not have feilds that require this.
		CaseSensitive:                 true,  // Case senstive field names. "Name" field in Struct will not match "name" in json string when deserializing
	}.Froze()

myOrganizationLoader := NewOrganizationLoader(OrganizationLoaderConfig{
	Wait:        5 * time.Millisecond,
	MaxBatch:    200,
	Fetch:       myOrganizationFetchFunc,
	RedisConfig: &OrganizationLoaderRedisConfig{
				SetTTL:         &ttl,
				GetFunc:        func(ctx context.Context, key string) (string, error) {
				    v, err := redisClient.Get(ctx, key).Result()
                	if err != nil && err == redis.Nil {
                		// no matching key found, empty result
                		return "", ErrRecordNotFound
                	} else if err != nil {
                		// error getting value from Redis
                		return "", err
                	}
                	return v, nil
				},
				GetManyFunc:     func(ctx context.Context, keys []string) ([]string, []error, error) {
				    v, err := redisClient.MGet(ctx, keys...).Result()
                	if err != nil {
                		// error getting value from Redis
                		return nil, nil, err
                	}
                	errs := make([]error, len(v))
                	ret := make([]string, len(v))
                	for i, v := range v {
                		if v == nil {
                			// key not found, set errs
                			errs[i] = ErrRecordNotFound
                		} else {
                			// key found, set value
                			switch typed := v.(type) {
                			case string:
                				ret[i] = typed
                			default:
                				errs[i] = ErrRecordNotFound
                			}
                		}
                	}
                	return ret, errs, nil
				},
				SetFunc:      func(ctx context.Context, key string, value interface{}, ttl *time.Duration) error {
					var v interface{}
                	if value == nil {
                		// empty value. Key's value in Redis will be "null", no quotes
                		v = nil
                	} else {
                		switch typed := value.(type) {
                		case string:
                			// value is already a string, use as is
                			v = typed
                		default:
                			// serialize interface to byte array
                			var err error
                			v, err = json.Marshal(value)
                			if err != nil {
                				// failed to serialize object
                				return err
                			}
                		}
                	}
                	if ttl == nil {
                		// use system default TTL
                		return redisClient.Set(ctx, key, v, 0).Err()
                	} else {
                		// use provided TTL
                		return redisClient.Set(ctx, key, v, *ttl).Err()
                	}
				},
				DeleteFunc:     func(ctx context.Context, key string) error {
				    return redisClient.Del(ctx, key).Err()
				},
				DeleteManyFunc: func(ctx context.Context, keys []string) error {
				    return redisClient.Del(ctx, keys...).Err()
				},
				GetKeysFunc:    func(ctx context.Context, pattern string) ([]string, error) {
				    return redisClient.Keys(ctx, pattern).Result()
				},
				ObjMarshal:     JSONItGraphObjects.Marshal,
				ObjUnmarshal:   JSONItGraphObjects.Unmarshal,
			},
})
```

If you're using session based dataloaders in combination with globally available dataloaders, you can reference your global dataloader within your session using hooks like the following example:

```
sessionBasedOrgLoader = NewOrganizationLoader(OrganizationLoaderConfig{
	Wait:              5 * time.Millisecond,
	MaxBatch:          200,
	Fetch:             myOrganizationLoader.LoadAll,
	HookAfterPrime:    myOrganizationLoader.ForcePrime,
	HookAfterClear:    myOrganizationLoader.Clear,
	HookAfterClearAll: myOrganizationLoader.ClearAll,
})
```

#### Benchmarks

This fork (new tests/benchmarks have been added):
```
example\> go test -bench . -benchmem
goos: windows
goarch: amd64
pkg: github.com/viocle/dataloaden/example
cpu: AMD Ryzen 9 5900X 12-Core Processor
BenchmarkLoader/caches-24                       27250312              44.55 ns/op            10 B/op          0 allocs/op
BenchmarkLoader/random_spread-24                 2902282              600.2 ns/op           387 B/op          4 allocs/op
BenchmarkLoader/concurrently-24                      100           14177922 ns/op          2808 B/op         46 allocs/op
BenchmarkLoaderStruct/caches-24                 21124952              56.86 ns/op            10 B/op          0 allocs/op
BenchmarkLoaderStruct/random_spread-24          20847738              56.86 ns/op            10 B/op          0 allocs/op
BenchmarkLoaderStruct/concurrently-24            1000000               1090 ns/op            21 B/op          6 allocs/op
BenchmarkLoaderExpires/caches-24                26666428              45.14 ns/op            10 B/op          0 allocs/op
BenchmarkLoaderExpires/random_spread-24          2787622              645.7 ns/op           407 B/op          5 allocs/op
BenchmarkLoaderExpires/concurrently-24               100           14729930 ns/op          3067 B/op         56 allocs/op
BenchmarkLoaderExternalCache/caches-24          24000540              46.44 ns/op            10 B/op          0 allocs/op
BenchmarkLoaderExternalCache/random_spread-24    2823822              612.3 ns/op           404 B/op          4 allocs/op
BenchmarkLoaderExternalCache/concurrently-24         100           15002093 ns/op          2597 B/op         46 allocs/op
PASS
ok      github.com/viocle/dataloaden/example    19.834s

example\slice\> go test -bench . -benchmem
goos: windows
goarch: amd64
pkg: github.com/viocle/dataloaden/example/slice
cpu: AMD Ryzen 9 5900X 12-Core Processor
BenchmarkSliceLoader/caches-24                  28080801              39.35 ns/op            25 B/op          0 allocs/op
BenchmarkSliceLoader/random_spread-24           31927885              40.56 ns/op            26 B/op          0 allocs/op
BenchmarkSliceLoader/concurrently-24                 757            1603486 ns/op           160 B/op          3 allocs/op
PASS
ok      github.com/viocle/dataloaden/example/slice      4.928s
```

Parent package (wth new struct key of type UserByIDAndOrg struct{ID string; OrgID string} example and benchmark in slice package added):
```
example\> go test -bench . -benchmem
pkg: github.com/vektah/dataloaden/example
cpu: AMD Ryzen 9 5900X 12-Core Processor
BenchmarkLoader/caches-24                       16564267              70.18 ns/op            26 B/op          1 allocs/op
BenchmarkLoader/random_spread-24                 2370126              685.5 ns/op           359 B/op          4 allocs/op
BenchmarkLoader/concurently-24                       100           15033075 ns/op          2578 B/op         52 allocs/op
BenchmarkLoaderStruct/caches-24                 12949425              78.80 ns/op            26 B/op          1 allocs/op
BenchmarkLoaderStruct/random_spread-24          14267574              78.47 ns/op            26 B/op          1 allocs/op
BenchmarkLoaderStruct/concurently-24              771639               1423 ns/op           181 B/op         16 allocs/op
PASS
ok      github.com/vektah/dataloaden/example    8.857s

example\slice\> go test -bench . -benchmem
goos: windows
goarch: amd64
pkg: github.com/vektah/dataloaden/example/slice
cpu: AMD Ryzen 9 5900X 12-Core Processor
BenchmarkSliceLoader/caches-24                  16997695              66.47 ns/op            50 B/op          1 allocs/op
BenchmarkSliceLoader/random_spread-24           16678016              62.33 ns/op            50 B/op          1 allocs/op
BenchmarkSliceLoader/concurently-24                  825            1525405 ns/op           471 B/op         13 allocs/op
PASS
ok      github.com/vektah/dataloaden/example/slice      5.051s
```