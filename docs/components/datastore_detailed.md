# Datastore Component

## Overview

The Frame datastore component provides a robust database integration layer built on top of [GORM](https://gorm.io). It supports multiple database connections, read/write separation, and multi-tenancy out of the box.

## Features

### 1. Multiple Database Support
- Primary (write) database configuration
- Read replica support
- Automatic connection pooling
- Connection lifecycle management

### 2. Multi-tenancy
- Row-level tenant isolation
- Automatic tenant context injection
- Tenant-aware queries
- Shared database architecture

### 3. Migration Management
- Automatic schema migrations
- Version tracking
- Rollback support
- Custom migration scripts

## Configuration

### Basic Setup
```go
mainDbOption := frame.Datastore(ctx, "postgres://user:secret@primary_server/service_db", false)
readDbOption := frame.Datastore(ctx, "postgres://user:secret@secondary_server/service_db", true)
service := frame.NewService("Data service", mainDbOption, readDbOption)
```

### Connection URL Format
```
postgres://[user]:[password]@[host]:[port]/[database]?[options]
```

Common options:
- `sslmode=disable|require|verify-ca|verify-full`
- `timezone=UTC`
- `pool_max_conns=10`

## Usage Examples

### 1. Basic CRUD Operations

```go
type User struct {
    ID        uint
    Name      string
    Email     string
    TenantID  string `gorm:"index"`
}

// Create
db := frame.GetDB(ctx)
user := User{Name: "John", Email: "john@example.com"}
result := db.Create(&user)

// Read
var user User
db.First(&user, 1) // Find user with id 1

// Update
db.Model(&user).Update("Name", "John Doe")

// Delete
db.Delete(&user)
```

### 2. Working with Transactions

```go
err := frame.GetDB(ctx).Transaction(func(tx *gorm.DB) error {
    // Perform multiple operations
    if err := tx.Create(&user).Error; err != nil {
        return err
    }
    
    if err := tx.Create(&userProfile).Error; err != nil {
        return err
    }
    
    return nil
})
```

### 3. Multi-tenant Queries

```go
// Tenant context is automatically injected
db := frame.GetDB(ctx)
var users []User
db.Find(&users) // Will automatically filter by tenant

// Override tenant context
db.WithContext(frame.WithTenant(ctx, "tenant-id")).Find(&users)
```

## Best Practices

1. **Always Use Context**
   ```go
   db := frame.GetDB(ctx) // Instead of accessing DB directly
   ```

2. **Handle Errors**
   ```go
   if err := db.Create(&user).Error; err != nil {
       return fmt.Errorf("failed to create user: %w", err)
   }
   ```

3. **Use Transactions for Multiple Operations**
   ```go
   tx := db.Begin()
   defer func() {
       if r := recover(); r != nil {
           tx.Rollback()
       }
   }()
   ```

4. **Implement Proper Indexes**
   ```go
   type Model struct {
       ID        uint      `gorm:"primarykey"`
       CreatedAt time.Time `gorm:"index"`
       TenantID  string    `gorm:"index"`
   }
   ```

## Migration Management

### Creating Migrations

```go
type Migration20240207 struct{}

func (m *Migration20240207) Up(db *gorm.DB) error {
    return db.AutoMigrate(&User{})
}

func (m *Migration20240207) Down(db *gorm.DB) error {
    return db.Migrator().DropTable(&User{})
}
```

### Registering Migrations

```go
migrator := frame.NewMigrator()
migrator.AddMigration(&Migration20240207{})
```

## Performance Optimization

1. **Use Indexes Wisely**
   - Create indexes for frequently queried fields
   - Consider composite indexes for multi-column queries
   - Monitor index usage

2. **Batch Operations**
   ```go
   db.CreateInBatches(&users, 100)
   ```

3. **Select Required Fields**
   ```go
   db.Select("name", "email").Find(&users)
   ```

4. **Preload Related Data**
   ```go
   db.Preload("Profile").Find(&users)
   ```

## Monitoring and Debugging

1. **Enable SQL Logging**
   ```go
   db.Debug().Find(&users)
   ```

2. **Monitor Connection Pool**
   ```go
   stats := db.DB().Stats()
   log.Printf("Open connections: %d", stats.OpenConnections)
   ```

3. **Set Statement Timeout**
   ```go
   db.Statement.Settings.Set("gorm:query_timeout", time.Second*5)
   ```
