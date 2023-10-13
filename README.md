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

#### Benchmarks

This fork (new tests/benchmarks have been added):
```
example\> go test -bench . -benchmem
goos: windows
goarch: amd64
pkg: github.com/viocle/dataloaden/example
cpu: AMD Ryzen 9 5900X 12-Core Processor
BenchmarkLoader/caches-24                       27249440              45.21 ns/op            10 B/op          0 allocs/op
BenchmarkLoader/random_spread-24                 2931494              591.8 ns/op           407 B/op          4 allocs/op
BenchmarkLoader/concurently-24                       100           14288143 ns/op         12532 B/op         52 allocs/op
BenchmarkLoaderStruct/caches-24                 19104612              57.94 ns/op            10 B/op          0 allocs/op
BenchmarkLoaderStruct/random_spread-24          20508367              58.00 ns/op            10 B/op          0 allocs/op
BenchmarkLoaderStruct/concurently-24             1000000               1083 ns/op            21 B/op          6 allocs/op
BenchmarkLoaderExpires/caches-24                25801900              45.90 ns/op            10 B/op          0 allocs/op
BenchmarkLoaderExpires/random_spread-24          2761660              638.1 ns/op           430 B/op          5 allocs/op
BenchmarkLoaderExpires/concurently-24                100           14028404 ns/op         12472 B/op         61 allocs/op
PASS
ok      github.com/viocle/dataloaden/example    14.168s

example\slice\> go test -bench . -benchmem
goos: windows
goarch: amd64
pkg: github.com/viocle/dataloaden/example/slice
cpu: AMD Ryzen 9 5900X 12-Core Processor
BenchmarkLoader/caches-24                       28177306              39.53 ns/op            26 B/op          0 allocs/op
BenchmarkLoader/random_spread-24                31447117              41.37 ns/op            26 B/op          0 allocs/op
BenchmarkLoader/concurently-24                       589            1732780 ns/op            936 B/op         4 allocs/op
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