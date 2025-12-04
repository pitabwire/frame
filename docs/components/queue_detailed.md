# Queue Component

## Overview

Frame's queue component provides a robust message queue system that supports multiple backend implementations while maintaining a consistent interface. It's designed for scalability and reliability in handling asynchronous operations.

## Features

### 1. Multiple Queue Backends
- In-memory queue
- NATS
- Google Cloud Pub/Sub
- Extensible interface for custom implementations

### 2. Message Handling
- Publisher/Subscriber pattern
- Message persistence
- Delivery guarantees
- Error handling and retries

### 3. Scalability
- Horizontal scaling
- Load balancing
- Concurrent processing
- Batch operations

## Configuration

### Basic Setup

```go
func main() {
    ctx := context.Background()
    
    // Register publisher
    pubOpt := frame.RegisterPublisher("notifications", "mem://notifications")
    
    // Register subscriber
    subOpt := frame.RegisterSubscriber(
        "notifications",
        "mem://notifications",
        5,
        &NotificationHandler{},
    )
    
    // Create service with queue options
    service := frame.NewService(
        "queue-service",
        pubOpt,
        subOpt,
    )
}
```

### Queue URLs

Different backend implementations use different URL formats:

1. **Memory Queue**
   ```
   mem://queue-name
   ```

2. **NATS**
   ```
   nats://host:port/subject
   ```

3. **Google Cloud Pub/Sub**
   ```
   gcppubsub://project-id/topic
   ```

## Usage Examples

### 1. Message Publishing

```go
type NotificationMessage struct {
    UserID      string    `json:"user_id"`
    Type        string    `json:"type"`
    Content     string    `json:"content"`
    CreatedAt   time.Time `json:"created_at"`
}

func sendNotification(ctx context.Context, svc *frame.Service, userID, content string) error {
    msg := NotificationMessage{
        UserID:    userID,
        Type:      "general",
        Content:   content,
        CreatedAt: time.Now(),
    }
    
    data, err := json.Marshal(msg)
    if err != nil {
        return fmt.Errorf("failed to marshal message: %w", err)
    }
    
    return  svc.Publish(ctx, "notifications", data)
}
```

### 2. Message Handling

```go
type NotificationHandler struct{}

func (h *NotificationHandler) Handle(ctx context.Context, msg []byte) error {
    var notification NotificationMessage
    if err := json.Unmarshal(msg, &notification); err != nil {
        return fmt.Errorf("failed to unmarshal message: %w", err)
    }
    
    // Process notification
    return processNotification(ctx, notification)
}

func processNotification(ctx context.Context, notification NotificationMessage) error {
    // Implementation
    return nil
}
```

### 3. Batch Processing

```go
type BatchHandler struct {
    batchSize int
    messages  [][]byte
    mu        sync.Mutex
}

func (h *BatchHandler) Handle(ctx context.Context, msg []byte) error {
    h.mu.Lock()
    h.messages = append(h.messages, msg)
    
    if len(h.messages) >= h.batchSize {
        messages := h.messages
        h.messages = nil
        h.mu.Unlock()
        return h.processBatch(ctx, messages)
    }
    
    h.mu.Unlock()
    return nil
}

func (h *BatchHandler) processBatch(ctx context.Context, messages [][]byte) error {
    // Batch processing implementation
    return nil
}
```

## Best Practices

### 1. Error Handling

```go
type Handler struct {
    maxRetries int
    backoff    time.Duration
}

func (h *Handler) Handle(ctx context.Context, msg []byte) error {
    var attempt int
    for {
        err := h.process(ctx, msg)
        if err == nil {
            return nil
        }
        
        attempt++
        if attempt >= h.maxRetries {
            return fmt.Errorf("max retries exceeded: %w", err)
        }
        
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(h.backoff * time.Duration(attempt)):
            continue
        }
    }
}
```

### 2. Message Validation

```go
type Message struct {
    ID        string    `json:"id"`
    Type      string    `json:"type"`
    Payload   []byte    `json:"payload"`
    Timestamp time.Time `json:"timestamp"`
}

func (m *Message) Validate() error {
    if m.ID == "" {
        return errors.New("message ID is required")
    }
    if m.Type == "" {
        return errors.New("message type is required")
    }
    if len(m.Payload) == 0 {
        return errors.New("message payload is required")
    }
    return nil
}
```

### 3. Context Usage

```go
func (h *Handler) Handle(ctx context.Context, msg []byte) error {
    // Add timeout
    ctx, cancel := context.WithTimeout(ctx, time.Second*30)
    defer cancel()
    
    // Add tracing
    ctx, span := tracer.Start(ctx, "process_message")
    defer span.End()
    
    return h.process(ctx, msg)
}
```

## Performance Optimization

### 1. Connection Pooling

```go
type Pool struct {
    conns chan *Connection
}

func NewPool(size int) *Pool {
    return &Pool{
        conns: make(chan *Connection, size),
    }
}

func (p *Pool) Get() (*Connection, error) {
    select {
    case conn := <-p.conns:
        return conn, nil
    default:
        return newConnection()
    }
}

func (p *Pool) Put(conn *Connection) {
    select {
    case p.conns <- conn:
    default:
        conn.Close()
    }
}
```

### 2. Message Compression

```go
func compressMessage(data []byte) ([]byte, error) {
    var buf bytes.Buffer
    w := gzip.NewWriter(&buf)
    
    if _, err := w.Write(data); err != nil {
        return nil, err
    }
    
    if err := w.Close(); err != nil {
        return nil, err
    }
    
    return buf.Bytes(), nil
}

func decompressMessage(data []byte) ([]byte, error) {
    r, err := gzip.NewReader(bytes.NewReader(data))
    if err != nil {
        return nil, err
    }
    defer r.Close()
    
    return ioutil.ReadAll(r)
}
```

### 3. Concurrent Processing

```go
type WorkerPool struct {
    workers int
    jobs    chan []byte
}

func NewWorkerPool(workers int) *WorkerPool {
    return &WorkerPool{
        workers: workers,
        jobs:    make(chan []byte),
    }
}

func (p *WorkerPool) Start(ctx context.Context, handler Handler) {
    for i := 0; i < p.workers; i++ {
        go func() {
            for {
                select {
                case msg := <-p.jobs:
                    handler.Handle(ctx, msg)
                case <-ctx.Done():
                    return
                }
            }
        }()
    }
}
```

## Monitoring

### 1. Metrics Collection

```go
type Metrics struct {
    publishedMessages   prometheus.Counter
    consumedMessages    prometheus.Counter
    processingDuration prometheus.Histogram
    errorCount         prometheus.Counter
}

func NewMetrics() *Metrics {
    return &Metrics{
        publishedMessages: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "queue_published_messages_total",
            Help: "Total number of published messages",
        }),
        // ... other metrics
    }
}
```

### 2. Health Checks

```go
func (s *Service) HealthCheck() error {
    ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
    defer cancel()
    
    // Publish test message
    err := s.Publish(ctx, "health-check", []byte("ping"))
    if err != nil {
        return fmt.Errorf("queue health check failed: %w", err)
    }
    
    return nil
}
```
