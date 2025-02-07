# Security Components

## Authentication

Frame provides a flexible authentication system that supports multiple authentication methods and can be easily extended to support custom authentication providers.

### Features

1. **Multiple Auth Methods**
   - OAuth2
   - JWT
   - Basic Auth
   - Custom providers

2. **Token Management**
   - Token validation
   - Token refresh
   - Token revocation
   - Claims handling

3. **Session Management**
   - Session creation
   - Session validation
   - Session expiration
   - Session storage

### Basic Setup

```go
func main() {
    // Create auth config
    authConfig := frame.AuthConfig{
        JWTSecret: []byte("your-secret-key"),
        TokenExpiration: time.Hour * 24,
        RefreshExpiration: time.Hour * 24 * 7,
    }
    
    // Create auth middleware
    authMiddleware := frame.NewAuthMiddleware(authConfig)
    
    // Add to router
    router.Use(authMiddleware.Handler)
}
```

### OAuth2 Configuration

```go
oauth2Config := &oauth2.Config{
    ClientID:     "your-client-id",
    ClientSecret: "your-client-secret",
    RedirectURL:  "http://localhost:8080/callback",
    Scopes: []string{
        "profile",
        "email",
    },
    Endpoint: oauth2.Endpoint{
        AuthURL:  "https://provider.com/o/oauth2/auth",
        TokenURL: "https://provider.com/o/oauth2/token",
    },
}

authConfig := frame.AuthConfig{
    OAuth2Config: oauth2Config,
}
```

### JWT Implementation

```go
func createToken(user *User) (string, error) {
    claims := &jwt.StandardClaims{
        ExpiresAt: time.Now().Add(time.Hour * 24).Unix(),
        IssuedAt:  time.Now().Unix(),
        Subject:   user.ID,
    }
    
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte("your-secret-key"))
}

func validateToken(tokenString string) (*jwt.Token, error) {
    return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return []byte("your-secret-key"), nil
    })
}
```

## Authorization

Frame's authorization system is built on a flexible role-based access control (RBAC) model that can be extended to support attribute-based access control (ABAC) when needed.

### Features

1. **Role Management**
   - Role definition
   - Role assignment
   - Role hierarchy
   - Role validation

2. **Permission Management**
   - Permission definition
   - Permission assignment
   - Permission checking
   - Resource-level permissions

3. **Policy Enforcement**
   - Policy definition
   - Policy evaluation
   - Policy caching
   - Policy updates

### Basic Setup

```go
func main() {
    // Create authorization config
    authzConfig := frame.AuthzConfig{
        RoleDefinitions: map[string][]string{
            "admin":  {"read", "write", "delete"},
            "editor": {"read", "write"},
            "viewer": {"read"},
        },
    }
    
    // Create authorization middleware
    authzMiddleware := frame.NewAuthzMiddleware(authzConfig)
    
    // Add to router
    router.Use(authzMiddleware.Handler)
}
```

### Role-Based Access Control

```go
// Define roles
type Role struct {
    Name        string
    Permissions []string
}

// Check permissions
func hasPermission(ctx context.Context, permission string) bool {
    user := getUserFromContext(ctx)
    role := getRoleByName(user.Role)
    
    for _, p := range role.Permissions {
        if p == permission {
            return true
        }
    }
    return false
}

// Middleware implementation
func requirePermission(permission string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !hasPermission(r.Context(), permission) {
                http.Error(w, "Forbidden", http.StatusForbidden)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

### Resource-Level Authorization

```go
type ResourcePolicy struct {
    Resource    string
    Action      string
    Roles       []string
    Conditions  []Condition
}

type Condition interface {
    Evaluate(ctx context.Context, resource interface{}) bool
}

func authorizeResource(ctx context.Context, resource string, action string) error {
    user := getUserFromContext(ctx)
    policy := getPolicyForResource(resource)
    
    if !policy.AllowsAction(action, user.Role) {
        return ErrUnauthorized
    }
    
    for _, condition := range policy.Conditions {
        if !condition.Evaluate(ctx, resource) {
            return ErrForbidden
        }
    }
    
    return nil
}
```

## Best Practices

### 1. Token Management

```go
func refreshToken(refreshToken string) (*TokenPair, error) {
    // Validate refresh token
    claims, err := validateRefreshToken(refreshToken)
    if err != nil {
        return nil, err
    }
    
    // Create new token pair
    accessToken, err := createAccessToken(claims.Subject)
    if err != nil {
        return nil, err
    }
    
    newRefreshToken, err := createRefreshToken(claims.Subject)
    if err != nil {
        return nil, err
    }
    
    return &TokenPair{
        AccessToken:  accessToken,
        RefreshToken: newRefreshToken,
    }, nil
}
```

### 2. Secure Password Handling

```go
func hashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
    return string(bytes), err
}

func checkPasswordHash(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}
```

### 3. Session Management

```go
type Session struct {
    ID        string
    UserID    string
    ExpiresAt time.Time
    Data      map[string]interface{}
}

func createSession(userID string) (*Session, error) {
    session := &Session{
        ID:        uuid.New().String(),
        UserID:    userID,
        ExpiresAt: time.Now().Add(24 * time.Hour),
        Data:      make(map[string]interface{}),
    }
    
    return session, saveSession(session)
}
```

### 4. Rate Limiting

```go
func rateLimitMiddleware(next http.Handler) http.Handler {
    limiter := rate.NewLimiter(rate.Every(time.Second), 100)
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !limiter.Allow() {
            http.Error(w, "Too many requests", http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

## Security Considerations

1. **HTTPS Only**
   ```go
   router.Use(func(next http.Handler) http.Handler {
       return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           if r.Header.Get("X-Forwarded-Proto") != "https" {
               http.Error(w, "HTTPS required", http.StatusBadRequest)
               return
           }
           next.ServeHTTP(w, r)
       })
   })
   ```

2. **Secure Headers**
   ```go
   func secureHeaders(next http.Handler) http.Handler {
       return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           w.Header().Set("X-XSS-Protection", "1; mode=block")
           w.Header().Set("X-Frame-Options", "DENY")
           w.Header().Set("X-Content-Type-Options", "nosniff")
           w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
           next.ServeHTTP(w, r)
       })
   })
   ```

3. **CSRF Protection**
   ```go
   func csrfMiddleware(next http.Handler) http.Handler {
       return csrf.Protect(
           []byte("32-byte-long-auth-key"),
           csrf.Secure(true),
           csrf.HttpOnly(true),
       )(next)
   }
   ```
