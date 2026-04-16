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
		cwd, _ := os.Getwd()
		log.Warn().Str("cwd", cwd).Msg("no migration files found")
		return
	}

	log.Info().Int("count", len(migrations)).Msg("found migration files")

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
	// Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Try multiple possible paths for migrations directory
	possiblePaths := []string{
		filepath.Join(cwd, "migrations"),             // ./migrations
		filepath.Join(cwd, "..", "migrations"),       // ../migrations
		filepath.Join(cwd, "..", "..", "migrations"), // ../../migrations
		"migrations", // relative path
	}

	var files []string
	var migrationsDir string

	// Find the migrations directory
	for _, path := range possiblePaths {
		pattern := filepath.Join(path, fmt.Sprintf("*_%s.sql", direction))
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			files = matches
			migrationsDir = path
			break
		}
	}

	if len(files) == 0 {
		// Last attempt: check if migrations directory exists and list its contents
		for _, path := range possiblePaths {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				entries, err := os.ReadDir(path)
				if err == nil {
					fmt.Printf("Found migrations directory at: %s\n", path)
					fmt.Printf("Contents:\n")
					for _, entry := range entries {
						fmt.Printf("  - %s\n", entry.Name())
					}
					migrationsDir = path
					break
				}
			}
		}

		if migrationsDir != "" {
			// Try to read files directly
			entries, err := os.ReadDir(migrationsDir)
			if err != nil {
				return nil, fmt.Errorf("failed to read migrations directory: %w", err)
			}

			suffix := fmt.Sprintf("_%s.sql", direction)
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
					files = append(files, filepath.Join(migrationsDir, entry.Name()))
				}
			}
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no migration files found in any of the expected locations")
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
