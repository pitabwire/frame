# Frame Cache Package

A powerful, generic cache implementation with automatic serialization and swappable backends for the Frame service framework.

## Features

- **Generic Type-Safe API**: Cache any Go type with automatic serialization
- **Multiple Backends**: In-memory, Redis, and Valkey implementations
- **Lazy Loading**: Redis and Valkey backends are in subpackages and only loaded when used
- **Automatic Serialization**: JSON and Gob serializers included, custom serializers supported
- **Flexible Keys**: Use any comparable type as cache keys
- **Thread-Safe**: All operations are concurrent-safe
- **TTL Support**: Automatic expiration of cached items
- **Prefix Support**: Namespace isolation for multi-tenancy
- **Raw Cache API**: Direct byte-level operations when needed

## Quick Start

```go
import "github.com/pitabwire/frame/cache"

// 1. Create manager
manager := cache.NewManager()
defer manager.Close()

// 2. Add in-memory cache
manager.AddCache("users", cache.NewInMemoryCache())

// 3. Get typed cache
type User struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}
userCache, _ := cache.GetCache[string, User](manager, "users", nil, nil)

// 4. Use it
ctx := context.Background()
user := User{ID: "123", Name: "John"}
userCache.Set(ctx, user.ID, user, 1*time.Hour)

// 5. Retrieve
cachedUser, found, _ := userCache.Get(ctx, "123")
```

## Architecture

```
cache/
├── cache.go           # Core interfaces and generic wrapper
├── inmemory.go        # In-memory implementation (no external deps)
├── redis/
│   └── redis.go       # Redis implementation (lazy loaded)
└── valkey/
    └── valkey.go      # Valkey implementation (lazy loaded)
```

### Key Components

#### `Cache[K, V]` - Generic Cache Interface
Type-safe cache interface with automatic serialization:
```go
type Cache[K comparable, V any] interface {
    Get(ctx context.Context, key K) (V, bool, error)
    Set(ctx context.Context, key K, value V, ttl time.Duration) error
    Delete(ctx context.Context, key K) error
    Exists(ctx context.Context, key K) (bool, error)
    Flush(ctx context.Context) error
    Close() error
}
```

#### `RawCache` - Low-Level Cache Interface
Direct byte-level operations:
```go
type RawCache interface {
    Get(ctx context.Context, key string) ([]byte, bool, error)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    // ... additional methods including Increment/Decrement
}
```

#### `Serializer` - Pluggable Serialization
```go
type Serializer interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
}
```

Built-in serializers:
- `JSONSerializer` - JSON encoding (default)
- `GobSerializer` - Go's gob encoding

## Usage

### Basic Usage with Service

```go
import (
    "github.com/pitabwire/frame"
    "github.com/pitabwire/frame/cache"
)

type User struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

// Create service with cache
ctx, svc := frame.NewService("myapp",
    frame.WithInMemoryCache("users"),
)

// Get cache manager from service
manager := svc.CacheManager()

// Get typed cache - automatic JSON serialization
userCache, ok := cache.GetCache[string, User](manager, "users", nil, nil)

// Store user
user := User{ID: "123", Name: "John", Email: "john@example.com"}
userCache.Set(ctx, user.ID, user, 1*time.Hour)

// Retrieve user
cachedUser, found, _ := userCache.Get(ctx, "123")
if found {
    fmt.Printf("User: %s\n", cachedUser.Name)
}
```

### Standalone Cache Manager

```go
import "github.com/pitabwire/frame/cache"

// Create manager
manager := cache.NewManager()
defer manager.Close()

// Add cache
manager.AddCache("products", cache.NewInMemoryCache())

// Get typed cache with int keys
type Product struct {
    ID    int     `json:"id"`
    Name  string  `json:"name"`
    Price float64 `json:"price"`
}

productCache, _ := cache.GetCache[int, Product](manager, "products", nil, nil)

// Use it
product := Product{ID: 1, Name: "Widget", Price: 9.99}
productCache.Set(ctx, product.ID, product, 1*time.Hour)
```

### Redis Cache (Lazy Loaded)

```go
import (
    "github.com/pitabwire/frame/cache"
    cacheredis "github.com/pitabwire/frame/cache/redis"
)

// Create Redis cache
redisCache, err := cacheredis.New(cacheredis.Options{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
})
if err != nil {
    log.Fatal(err)
}

manager := cache.NewManager()
manager.AddCache("redis", redisCache)

// Get typed cache
userCache, _ := cache.GetCache[string, User](manager, "redis", nil, nil)
```

### Valkey Cache (Lazy Loaded)

```go
import (
    "github.com/pitabwire/frame/cache"
    cachevalkey "github.com/pitabwire/frame/cache/valkey"
)

// Create Valkey cache
valkeyCache, err := cachevalkey.New(cachevalkey.Options{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
})
if err != nil {
    log.Fatal(err)
}

manager := cache.NewManager()
manager.AddCache("valkey", valkeyCache)

// Get typed cache with custom key function
productCache, _ := cache.GetCache[int, Product](
    manager,
    "valkey",
    nil,
    func(id int) string { return fmt.Sprintf("product:%d", id) },
)
```

### Custom Serializer

```go
// Use Gob serializer instead of JSON
userCache, _ := cache.GetCache[string, User](
    manager,
    "users",
    &cache.GobSerializer{},
    nil,
)
```

### Custom Key Function

```go
// Custom key formatting
userCache, _ := cache.GetCache[string, User](
    manager,
    "users",
    nil,
    func(userID string) string {
        return fmt.Sprintf("user:%s", userID)
    },
)
```

### Raw Cache Operations

```go
// Get raw cache for byte-level operations
rawCache, _ := manager.GetRawCache("users")

// Direct byte operations
rawCache.Set(ctx, "binary:data", []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f}, 0)

// Counter operations
count, _ := rawCache.Increment(ctx, "page:views", 1)
fmt.Printf("Page views: %d\n", count)
```

## Dependency Management

The cache package is designed to minimize dependencies:

- **Core Cache Package** (`cache/`) - No external dependencies except standard library
- **In-Memory Cache** - No external dependencies
- **Redis Cache** (`cache/redis/`) - Only loaded when imported, requires `github.com/redis/go-redis/v9`
- **Valkey Cache** (`cache/valkey/`) - Only loaded when imported, requires `github.com/valkey-io/valkey-go`

### Example go.mod

If you only use in-memory cache:
```go
require (
    github.com/pitabwire/frame v1.0.0
    // No redis dependency needed!
)
```

If you use Redis:
```go
require (
    github.com/pitabwire/frame v1.0.0
    github.com/redis/go-redis/v9 v9.14.1  // Only included if you import cache/redis
)
```

If you use Valkey:
```go
require (
    github.com/pitabwire/frame v1.0.0
    github.com/valkey-io/valkey-go v1.0.67  // Only included if you import cache/valkey
)
```

## Type Safety Examples

### String Keys, Custom Values

```go
type Session struct {
    UserID    string    `json:"user_id"`
    ExpiresAt time.Time `json:"expires_at"`
}

sessionCache, _ := cache.GetCache[string, Session](manager, "sessions", nil, nil)
```

### Int Keys, Struct Values

```go
type Product struct {
    ID          int     `json:"id"`
    Name        string  `json:"name"`
    Price       float64 `json:"price"`
    InStock     bool    `json:"in_stock"`
}

productCache, _ := cache.GetCache[int, Product](manager, "products", nil, nil)
```

### UUID Keys

```go
import "github.com/google/uuid"

type Order struct {
    ID     uuid.UUID `json:"id"`
    Total  float64   `json:"total"`
    Status string    `json:"status"`
}

orderCache, _ := cache.GetCache[uuid.UUID, Order](
    manager,
    "orders",
    nil,
    func(id uuid.UUID) string { return id.String() },
)
```

### Nested Structures

```go
type Address struct {
    Street  string `json:"street"`
    City    string `json:"city"`
    Country string `json:"country"`
}

type Customer struct {
    ID      string  `json:"id"`
    Name    string  `json:"name"`
    Address Address `json:"address"`
}

customerCache, _ := cache.GetCache[string, Customer](manager, "customers", nil, nil)
```

## Performance Considerations

### In-Memory Cache
- **Pros**: Zero latency, no network overhead, no external dependencies
- **Cons**: Limited by RAM, not distributed, lost on restart
- **Best for**: Small datasets, development, single-instance apps, hot data

### Redis Cache  
- **Pros**: Persistent, distributed, scales horizontally, battle-tested
- **Cons**: Network latency, requires Redis server, external dependency
- **Best for**: Large datasets, production, multi-instance apps, shared cache

### Valkey Cache
- **Pros**: Redis-compatible, open-source, uses official valkey-go client, community-driven
- **Cons**: Network latency, requires Valkey server, external dependency
- **Best for**: Same as Redis, when you prefer open-source alternatives with native client support

### Optimization Tips

1. **Choose appropriate serializers**: JSON for interoperability, Gob for Go-only performance
2. **Set reasonable TTLs** to prevent unbounded growth
3. **Use raw cache** for simple byte operations (skip serialization overhead)
4. **Customize key functions** to optimize key formatting
5. **Use appropriate cache backend**: In-memory for hot data, Redis/Valkey for shared/persistent cache

## Testing

The cache package includes comprehensive tests with real container integration:

```bash
# Run all cache tests (includes InMemory, Redis, and Valkey with real containers)
go test -v ./cache/

# Test specific implementations
go test -v ./cache/redis/    # Requires Docker
go test -v ./cache/valkey/   # Requires Docker
```

All implementations run through the same unified test suite, ensuring consistent behavior across backends.

## Best Practices

1. **Use generics for type safety**: Leverage `Cache[K, V]` instead of raw bytes
2. **Choose appropriate serializers**: JSON for external systems, Gob for internal
3. **Lazy load backends**: Only import redis/valkey when needed
4. **Use raw cache sparingly**: Only for counters and binary data
5. **Set TTLs appropriately**: Balance freshness vs. load
6. **Handle cache misses gracefully**: Always check the `found` boolean
7. **Close caches properly**: Always defer `cache.Close()` or `manager.Close()`
8. **Use appropriate cache backends**: In-memory for development, Redis/Valkey for production

## License

Part of the Frame service framework.
