package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/swiftmail/swiftmail/internal/config"
	"github.com/swiftmail/swiftmail/pkg/logger"
)

func main() {
	direction := flag.String("direction", "up", "Migration direction: up or down")
	flag.Parse()

	cfg := config.Load()
	log := logger.New(cfg.App.Env)

	ctx := context.Background()

	db, err := pgxpool.New(ctx, cfg.Database.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	// Get all migration files
	migrations, err := getMigrationFiles(*direction)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get migration files")
	}

	if len(migrations) == 0 {
		log.Warn().Msg("no migration files found")
		return
	}

	// Run migrations in order
	for _, filename := range migrations {
		// Skip ClickHouse migrations (they need separate handling)
		if strings.Contains(filename, "clickhouse") {
			log.Info().Str("file", filename).Msg("skipping ClickHouse migration (run separately)")
			continue
		}

		log.Info().Str("file", filename).Msg("running migration")

		sql, err := os.ReadFile(filename)
		if err != nil {
			log.Fatal().Err(err).Str("file", filename).Msg("failed to read migration file")
		}

		_, err = db.Exec(ctx, string(sql))
		if err != nil {
			log.Fatal().Err(err).Str("file", filename).Msg("migration failed")
		}

		fmt.Printf("✓ Migration completed: %s\n", filepath.Base(filename))
	}

	fmt.Printf("\n✓ All PostgreSQL migrations %s completed successfully\n", *direction)
	log.Info().Str("direction", *direction).Int("count", len(migrations)).Msg("all migrations completed")
}

// getMigrationFiles returns sorted list of migration files for the given direction
func getMigrationFiles(direction string) ([]string, error) {
	pattern := fmt.Sprintf("migrations/*_%s.sql", direction)
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	// Sort files to ensure correct order
	sort.Strings(files)

	// If direction is down, reverse the order
	if direction == "down" {
		for i := len(files)/2 - 1; i >= 0; i-- {
			opp := len(files) - 1 - i
			files[i], files[opp] = files[opp], files[i]
		}
	}

	return files, nil
}
