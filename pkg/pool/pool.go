package pool

import (
	"errors"
	"sync"
	"time"
)

// ErrPoolClosed is returned when attempting to use a closed pool.
var ErrPoolClosed = errors.New("pool is closed")

// ErrPoolExhausted is returned when no connections are available.
var ErrPoolExhausted = errors.New("pool exhausted")

// Connection represents a generic poolable connection.
type Connection interface {
	Close() error
	IsAlive() bool
}

// Factory creates new connections.
type Factory func() (Connection, error)

// Pool is a generic connection pool with pre-warming support.
type Pool struct {
	mu       sync.Mutex
	conns    chan Connection
	factory  Factory
	maxSize  int
	closed   bool
	created  int
	metrics  PoolMetrics
}

// PoolMetrics tracks pool usage statistics.
type PoolMetrics struct {
	TotalCreated  int64
	TotalReused   int64
	TotalClosed   int64
	ActiveCount   int
}

// Config for pool initialization.
type Config struct {
	MaxSize    int
	PreWarm    int
	Factory    Factory
}

// New creates a new connection pool.
func New(cfg Config) (*Pool, error) {
	p := &Pool{
		conns:   make(chan Connection, cfg.MaxSize),
		factory: cfg.Factory,
		maxSize: cfg.MaxSize,
	}

	// Pre-warm connections
	for i := 0; i < cfg.PreWarm; i++ {
		conn, err := cfg.Factory()
		if err != nil {
			// Non-fatal: log warning but continue
			continue
		}
		p.conns <- conn
		p.created++
		p.metrics.TotalCreated++
	}

	return p, nil
}

// Get retrieves a connection from the pool, creating a new one if necessary.
func (p *Pool) Get() (Connection, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}
	p.mu.Unlock()

	// Try to get an existing connection (non-blocking)
	select {
	case conn := <-p.conns:
		if conn.IsAlive() {
			p.mu.Lock()
			p.metrics.TotalReused++
			p.metrics.ActiveCount++
			p.mu.Unlock()
			return conn, nil
		}
		// Connection is dead, close it and create a new one
		conn.Close()
		p.mu.Lock()
		p.metrics.TotalClosed++
		p.created--
		p.mu.Unlock()
	default:
		// No available connection
	}

	// Create a new connection if under limit
	p.mu.Lock()
	if p.created >= p.maxSize {
		p.mu.Unlock()
		// Wait for a connection to be returned
		select {
		case conn := <-p.conns:
			if conn.IsAlive() {
				p.mu.Lock()
				p.metrics.TotalReused++
				p.metrics.ActiveCount++
				p.mu.Unlock()
				return conn, nil
			}
			conn.Close()
			p.mu.Lock()
			p.metrics.TotalClosed++
			p.created--
			p.mu.Unlock()
			return p.createNew()
		case <-time.After(10 * time.Second):
			return nil, ErrPoolExhausted
		}
	}
	p.mu.Unlock()

	return p.createNew()
}

func (p *Pool) createNew() (Connection, error) {
	conn, err := p.factory()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.created++
	p.metrics.TotalCreated++
	p.metrics.ActiveCount++
	p.mu.Unlock()
	return conn, nil
}

// Put returns a connection to the pool.
func (p *Pool) Put(conn Connection) {
	p.mu.Lock()
	p.metrics.ActiveCount--
	if p.closed {
		p.mu.Unlock()
		conn.Close()
		return
	}
	p.mu.Unlock()

	if !conn.IsAlive() {
		conn.Close()
		p.mu.Lock()
		p.metrics.TotalClosed++
		p.created--
		p.mu.Unlock()
		return
	}

	select {
	case p.conns <- conn:
		// Returned to pool
	default:
		// Pool is full, close the connection
		conn.Close()
		p.mu.Lock()
		p.metrics.TotalClosed++
		p.created--
		p.mu.Unlock()
	}
}

// Close closes all connections in the pool.
func (p *Pool) Close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()

	close(p.conns)
	for conn := range p.conns {
		conn.Close()
	}
}

// Stats returns current pool statistics.
func (p *Pool) Stats() PoolMetrics {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.metrics
}

// Size returns the number of connections currently in the pool (available).
func (p *Pool) Size() int {
	return len(p.conns)
}
