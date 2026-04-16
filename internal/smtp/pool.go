package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	smtplib "net/smtp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/swiftmail/swiftmail/internal/config"
)

// Pool manages a pool of SMTP connections for high-throughput delivery.
type Pool struct {
	mu              sync.RWMutex
	connections     chan *Connection
	factory         func() (*Connection, error)
	minSize         int
	maxSize         int
	created         int32
	closed          bool
	logger          zerolog.Logger
	smtpHost        string
	smtpPort        string
	smtpUsername    string
	smtpPassword    string
	tlsConfig       *tls.Config
	connectTimeout  time.Duration
	sendTimeout     time.Duration
	maxIdleTime     time.Duration
	maxConnAge      time.Duration
	maxConnUses     int
	healthCheckTick *time.Ticker
	ctx             context.Context
	cancel          context.CancelFunc
}

// Connection wraps a single SMTP connection.
type Connection struct {
	client      *smtplib.Client
	createdAt   time.Time
	lastUsedAt  time.Time
	usedCount   int32
	alive       bool
	mu          sync.Mutex
	maxUses     int
	maxAge      time.Duration
	maxIdleTime time.Duration
}

// NewPool creates a pre-warmed SMTP connection pool with dynamic scaling.
func NewPool(cfg *config.SMTPConfig, logger zerolog.Logger) (*Pool, error) {
	ctx, cancel := context.WithCancel(context.Background())

	p := &Pool{
		connections:     make(chan *Connection, cfg.MaxPoolSize),
		minSize:         cfg.MinPoolSize,
		maxSize:         cfg.MaxPoolSize,
		logger:          logger,
		smtpHost:        cfg.Host,
		smtpPort:        cfg.Port,
		smtpUsername:    cfg.Username,
		smtpPassword:    cfg.Password,
		connectTimeout:  cfg.ConnectTimeout,
		sendTimeout:     cfg.SendTimeout,
		maxIdleTime:     cfg.MaxIdleTime,
		maxConnAge:      cfg.MaxConnAge,
		maxConnUses:     cfg.MaxConnUses,
		healthCheckTick: time.NewTicker(30 * time.Second),
		ctx:             ctx,
		cancel:          cancel,
		tlsConfig: &tls.Config{
			ServerName:         cfg.Host,
			InsecureSkipVerify: cfg.Host == "localhost" || cfg.Host == "127.0.0.1", // Skip verification for localhost
		},
	}

	p.factory = p.createConnection

	// Pre-warm minimum connections
	warmed := 0
	for i := 0; i < cfg.MinPoolSize; i++ {
		conn, err := p.factory()
		if err != nil {
			logger.Warn().Err(err).Int("attempt", i).Msg("failed to pre-warm SMTP connection")
			continue
		}
		p.connections <- conn
		atomic.AddInt32(&p.created, 1)
		warmed++
	}

	logger.Info().
		Int("warmed", warmed).
		Int("min_size", cfg.MinPoolSize).
		Int("max_size", cfg.MaxPoolSize).
		Msg("SMTP connection pool initialized")

	// Start background health checker
	go p.healthChecker()

	return p, nil
}

func (p *Pool) createConnection() (*Connection, error) {
	addr := net.JoinHostPort(p.smtpHost, p.smtpPort)

	// Connect with timeout
	netConn, err := net.DialTimeout("tcp", addr, p.connectTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	client, err := smtplib.NewClient(netConn, p.smtpHost)
	if err != nil {
		netConn.Close()
		return nil, fmt.Errorf("failed to create SMTP client: %w", err)
	}

	// Try STARTTLS
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(p.tlsConfig); err != nil {
			p.logger.Warn().Err(err).Msg("STARTTLS failed, continuing without TLS")
		}
	}

	// Authenticate if credentials provided
	if p.smtpUsername != "" && p.smtpPassword != "" {
		auth := smtplib.PlainAuth("", p.smtpUsername, p.smtpPassword, p.smtpHost)
		if err := client.Auth(auth); err != nil {
			client.Close()
			netConn.Close()
			return nil, fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	now := time.Now()
	return &Connection{
		client:      client,
		createdAt:   now,
		lastUsedAt:  now,
		alive:       true,
		maxUses:     p.maxConnUses,
		maxAge:      p.maxConnAge,
		maxIdleTime: p.maxIdleTime,
	}, nil
}

// Get retrieves a connection from the pool with timeout support.
// OPTIMIZATION: Wait for available connection instead of failing immediately
func (p *Pool) Get() (*Connection, error) {
	return p.GetWithTimeout(context.Background(), 5*time.Second)
}

// GetWithTimeout retrieves a connection from the pool with configurable timeout.
func (p *Pool) GetWithTimeout(ctx context.Context, timeout time.Duration) (*Connection, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, fmt.Errorf("pool is closed")
	}
	p.mu.RUnlock()

	deadline := time.Now().Add(timeout)

	for {
		// Fast path: try to get an existing connection (non-blocking)
		select {
		case conn := <-p.connections:
			if conn.IsAlive() {
				atomic.AddInt32(&conn.usedCount, 1)
				conn.lastUsedAt = time.Now()
				return conn, nil
			}
			// Dead connection, close and create new one
			conn.Close()
			atomic.AddInt32(&p.created, -1)
		default:
			// No available connections, try to create new one
		}

		// Check if we can create a new connection
		current := atomic.LoadInt32(&p.created)
		if int(current) < p.maxSize {
			// Try to increment size atomically
			if atomic.CompareAndSwapInt32(&p.created, current, current+1) {
				conn, err := p.factory()
				if err != nil {
					atomic.AddInt32(&p.created, -1)
					return nil, err
				}
				atomic.AddInt32(&conn.usedCount, 1)
				conn.lastUsedAt = time.Now()
				return conn, nil
			}
		}

		// Pool is at max capacity, check timeout
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("pool exhausted: %d/%d connections in use (timeout after %v)", current, p.maxSize, timeout)
		}

		// Wait a bit before retrying (exponential backoff would be better, but this is simpler)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// Retry
		}
	}
}

// Put returns a connection to the pool.
func (p *Pool) Put(conn *Connection) {
	if !conn.IsAlive() {
		conn.Close()
		atomic.AddInt32(&p.created, -1)
		return
	}

	// Try to return to pool (non-blocking)
	select {
	case p.connections <- conn:
		// Successfully returned to pool
	default:
		// Pool is full, close the connection
		conn.Close()
		atomic.AddInt32(&p.created, -1)
	}
}

// Close shuts down the pool and closes all connections.
func (p *Pool) Close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()

	p.cancel()
	p.healthCheckTick.Stop()

	close(p.connections)
	for conn := range p.connections {
		conn.Close()
	}

	p.logger.Info().Msg("SMTP pool closed")
}

// healthChecker runs periodic health checks on idle connections.
func (p *Pool) healthChecker() {
	for {
		select {
		case <-p.healthCheckTick.C:
			p.performHealthCheck()
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *Pool) performHealthCheck() {
	// Check pool size and maintain minimum
	current := atomic.LoadInt32(&p.created)
	available := len(p.connections)

	p.logger.Debug().
		Int32("current_size", current).
		Int("available", available).
		Int("min_size", p.minSize).
		Msg("pool health check")

	// If below minimum, create more connections
	if int(current) < p.minSize {
		needed := p.minSize - int(current)
		for i := 0; i < needed; i++ {
			conn, err := p.factory()
			if err != nil {
				p.logger.Warn().Err(err).Msg("failed to create connection during health check")
				continue
			}
			p.connections <- conn
			atomic.AddInt32(&p.created, 1)
		}
	}

	// Clean up stale connections
	toCheck := available
	for i := 0; i < toCheck; i++ {
		select {
		case conn := <-p.connections:
			if conn.IsAlive() {
				p.connections <- conn // Return healthy connection
			} else {
				conn.Close()
				atomic.AddInt32(&p.created, -1)
			}
		default:
			return
		}
	}
}

// Stats returns pool statistics.
func (p *Pool) Stats() (available, total, min, max int) {
	return len(p.connections), int(atomic.LoadInt32(&p.created)), p.minSize, p.maxSize
}

// IsAlive checks if the connection is still usable.
func (c *Connection) IsAlive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.alive {
		return false
	}

	now := time.Now()

	// Check if connection is too old
	if now.Sub(c.createdAt) > c.maxAge {
		c.alive = false
		return false
	}

	// Check if connection has been idle too long
	if now.Sub(c.lastUsedAt) > c.maxIdleTime {
		c.alive = false
		return false
	}

	// Check if connection has been used too many times
	if atomic.LoadInt32(&c.usedCount) >= int32(c.maxUses) {
		c.alive = false
		return false
	}

	// NOOP check (lightweight health check)
	if c.client != nil {
		if err := c.client.Noop(); err != nil {
			c.alive = false
			return false
		}
	}

	return true
}

// Close closes the SMTP connection.
func (c *Connection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.alive = false
	if c.client != nil {
		c.client.Quit()
	}
}

// SendMail sends an email using this connection.
func (c *Connection) SendMail(from string, to string, msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.alive || c.client == nil {
		return fmt.Errorf("connection is not alive")
	}

	// Reset for new message
	if err := c.client.Reset(); err != nil {
		c.alive = false
		return fmt.Errorf("SMTP RESET failed: %w", err)
	}

	if err := c.client.Mail(from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}

	if err := c.client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO failed: %w", err)
	}

	w, err := c.client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("SMTP write failed: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("SMTP close data failed: %w", err)
	}

	return nil
}

// extractSMTPCode extracts the status code from an SMTP error message.
func extractSMTPCode(err error) string {
	if err == nil {
		return "250"
	}
	msg := err.Error()
	if len(msg) >= 3 {
		code := msg[:3]
		if code[0] >= '2' && code[0] <= '5' {
			return code
		}
	}
	return ""
}

// isRecipientDomain extracts domain from "user@domain.com"
func isRecipientDomain(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}
