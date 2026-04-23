package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier interface defines database operations that work with both Pool and pgxpool.Pool
type Querier interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	Ping(ctx context.Context) error
}

// Pool wraps primary and read replica database connections
type Pool struct {
	Primary     *pgxpool.Pool
	ReadReplica *pgxpool.Pool
}

// NewPool creates a new database pool with optional read replica
func NewPool(ctx context.Context, primaryDSN, replicaDSN string) (*Pool, error) {
	// Connect to primary
	primary, err := pgxpool.New(ctx, primaryDSN)
	if err != nil {
		return nil, err
	}

	// Connect to read replica (or use primary if no replica configured)
	var replica *pgxpool.Pool
	if replicaDSN != "" && replicaDSN != primaryDSN {
		replica, err = pgxpool.New(ctx, replicaDSN)
		if err != nil {
			primary.Close()
			return nil, err
		}
	} else {
		replica = primary // Use primary for reads if no replica
	}

	return &Pool{
		Primary:     primary,
		ReadReplica: replica,
	}, nil
}

// Close closes both primary and replica connections
func (p *Pool) Close() {
	if p.ReadReplica != p.Primary {
		p.ReadReplica.Close()
	}
	p.Primary.Close()
}

// Ping pings both databases
func (p *Pool) Ping(ctx context.Context) error {
	if err := p.Primary.Ping(ctx); err != nil {
		return err
	}
	if p.ReadReplica != p.Primary {
		return p.ReadReplica.Ping(ctx)
	}
	return nil
}

// Query executes a query on the read replica (SELECT queries)
func (p *Pool) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return p.ReadReplica.Query(ctx, sql, args...)
}

// QueryRow executes a query on the read replica that returns a single row
func (p *Pool) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return p.ReadReplica.QueryRow(ctx, sql, args...)
}

// Exec executes a command on the primary database (INSERT/UPDATE/DELETE)
func (p *Pool) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return p.Primary.Exec(ctx, sql, args...)
}

// Begin starts a transaction on the primary database
func (p *Pool) Begin(ctx context.Context) (pgx.Tx, error) {
	return p.Primary.Begin(ctx)
}

// BeginTx starts a transaction with options on the primary database
func (p *Pool) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	return p.Primary.BeginTx(ctx, txOptions)
}

// Acquire gets a connection from the primary pool
func (p *Pool) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
	return p.Primary.Acquire(ctx)
}

// AcquireRead gets a connection from the read replica pool
func (p *Pool) AcquireRead(ctx context.Context) (*pgxpool.Conn, error) {
	return p.ReadReplica.Acquire(ctx)
}

// GetPrimary returns the primary pool for components that need direct access
func (p *Pool) GetPrimary() *pgxpool.Pool {
	return p.Primary
}

// GetReadReplica returns the read replica pool for components that need direct access
func (p *Pool) GetReadReplica() *pgxpool.Pool {
	return p.ReadReplica
}
