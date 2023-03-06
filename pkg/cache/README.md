# Package `cache`

The `cache` package will supply you with caching solutions for your `crud` port-based resources.

You can swap `cache.Cache` with the resource you wish to cache,
and just set your original resource as the `Cache.Source`.
It is advised that your domain logic will be unaware of the caching value type.

```go
package main

import (
	"context"
	"domainpkg"
	"adapters/slowsolution"
	"adapters/fastsolution"
)

func main() {
	var repo domainpkg.XYRepository
	repo = slowsolution.NewXYRepository()
	repo = cache.New(repo, fastsolution.NewCacheRepository())
	bl := domainpkg.NewBusinessLogic(repo)
	_ = bl.Do(context.Background())
}
```
