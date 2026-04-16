package smtp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// MXResolver resolves MX records with Redis caching.
type MXResolver struct {
	rdb    *redis.Client
	logger zerolog.Logger
	ttl    time.Duration
}

// NewMXResolver creates a new MX resolver with caching.
func NewMXResolver(rdb *redis.Client, logger zerolog.Logger) *MXResolver {
	return &MXResolver{
		rdb:    rdb,
		logger: logger,
		ttl:    1 * time.Hour,
	}
}

// MXRecord represents a cached MX record.
type MXRecord struct {
	Host string `json:"host"`
	Pref uint16 `json:"pref"`
}

// Resolve returns MX records for a domain, using Redis cache when possible.
func (r *MXResolver) Resolve(ctx context.Context, domain string) ([]MXRecord, error) {
	cacheKey := fmt.Sprintf("mx:%s", domain)

	// Try cache first
	cached, err := r.rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		var records []MXRecord
		if err := json.Unmarshal([]byte(cached), &records); err == nil {
			return records, nil
		}
	}

	// DNS lookup
	mxRecords, err := net.LookupMX(domain)
	if err != nil {
		return nil, fmt.Errorf("MX lookup failed for %s: %w", domain, err)
	}

	if len(mxRecords) == 0 {
		return nil, fmt.Errorf("no MX records found for %s", domain)
	}

	// Sort by preference (lower = higher priority)
	sort.Slice(mxRecords, func(i, j int) bool {
		return mxRecords[i].Pref < mxRecords[j].Pref
	})

	records := make([]MXRecord, len(mxRecords))
	for i, mx := range mxRecords {
		records[i] = MXRecord{
			Host: mx.Host,
			Pref: mx.Pref,
		}
	}

	// Cache in Redis
	data, _ := json.Marshal(records)
	r.rdb.Set(ctx, cacheKey, string(data), r.ttl)

	return records, nil
}
