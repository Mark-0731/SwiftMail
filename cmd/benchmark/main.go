package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// BenchmarkConfig holds the test configuration
type BenchmarkConfig struct {
	RedisAddr       string
	RedisPassword   string
	WorkerCount     int
	MinDelayMs      int
	MaxDelayMs      int
	TestDurationSec int
}

// TaskPayload simulates email send payload
type TaskPayload struct {
	ID      string `json:"id"`
	To      string `json:"to"`
	From    string `json:"from"`
	Subject string `json:"subject"`
}

// BenchmarkStats tracks performance metrics
type BenchmarkStats struct {
	mu             sync.Mutex
	StartTime      time.Time
	TasksProcessed int64
	TasksFailed    int64
	MinDuration    time.Duration
	MaxDuration    time.Duration
	TotalDuration  time.Duration
}

func (s *BenchmarkStats) RecordTask(duration time.Duration, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if success {
		s.TasksProcessed++
		s.TotalDuration += duration

		if s.MinDuration == 0 || duration < s.MinDuration {
			s.MinDuration = duration
		}
		if duration > s.MaxDuration {
			s.MaxDuration = duration
		}
	} else {
		s.TasksFailed++
	}
}

func (s *BenchmarkStats) GetResults() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	elapsed := time.Since(s.StartTime).Seconds()
	var tasksPerSec float64
	var avgDuration float64

	if elapsed > 0 {
		tasksPerSec = float64(s.TasksProcessed) / elapsed
	}

	if s.TasksProcessed > 0 {
		avgDuration = float64(s.TotalDuration.Milliseconds()) / float64(s.TasksProcessed)
	}

	return map[string]interface{}{
		"timestamp_start":  s.StartTime.UTC().Format(time.RFC3339),
		"timestamp_end":    time.Now().UTC().Format(time.RFC3339),
		"duration_seconds": int(elapsed),
		"tasks_processed":  s.TasksProcessed,
		"tasks_failed":     s.TasksFailed,
		"tasks_per_second": tasksPerSec,
		"min_duration_ms":  s.MinDuration.Milliseconds(),
		"max_duration_ms":  s.MaxDuration.Milliseconds(),
		"avg_duration_ms":  avgDuration,
		"worker_count":     0, // Will be set by caller
	}
}

func main() {
	// Configuration based on actual SMTP send time (25ms average)
	config := BenchmarkConfig{
		RedisAddr:       "127.0.0.1:6379",
		RedisPassword:   "Mark@0731",
		WorkerCount:     200, // Test with 200 workers
		MinDelayMs:      10,  // Min SMTP time from logs
		MaxDelayMs:      76,  // Max SMTP time from logs
		TestDurationSec: 60,  // 1 minute test
	}

	fmt.Println("🚀 SwiftMail Queue Benchmark (Simulation)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Workers: %d\n", config.WorkerCount)
	fmt.Printf("Simulated delay: %d-%dms\n", config.MinDelayMs, config.MaxDelayMs)
	fmt.Printf("Test duration: %d seconds\n", config.TestDurationSec)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Initialize Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       0,
	})

	// Test Redis connection
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	fmt.Println("✅ Connected to Redis")

	// Initialize Asynq client (for queuing tasks)
	client := asynq.NewClient(asynq.RedisClientOpt{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
	})
	defer client.Close()

	// Initialize stats
	stats := &BenchmarkStats{
		StartTime: time.Now(),
	}

	// Create task handler
	handler := func(ctx context.Context, t *asynq.Task) error {
		start := time.Now()

		// Simulate processing delay based on your logs
		delayMs := config.MinDelayMs + rand.Intn(config.MaxDelayMs-config.MinDelayMs)
		time.Sleep(time.Duration(delayMs) * time.Millisecond)

		duration := time.Since(start)
		stats.RecordTask(duration, true)

		return nil
	}

	// Initialize Asynq server (worker)
	srv := asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     config.RedisAddr,
			Password: config.RedisPassword,
		},
		asynq.Config{
			Concurrency: config.WorkerCount,
			Queues: map[string]int{
				"default": 1,
			},
		},
	)

	// Start worker in background
	mux := asynq.NewServeMux()
	mux.HandleFunc("benchmark:task", handler)

	go func() {
		if err := srv.Run(mux); err != nil {
			log.Fatalf("Worker error: %v", err)
		}
	}()

	fmt.Printf("✅ Worker started with %d concurrent workers\n", config.WorkerCount)
	time.Sleep(1 * time.Second) // Let worker initialize

	// Queue tasks continuously
	fmt.Println("📤 Queuing tasks...")
	go func() {
		count := 0
		for {
			count++
			payload := TaskPayload{
				ID:      fmt.Sprintf("task-%d", count),
				To:      "test@localhost",
				From:    "benchmark@localhost",
				Subject: fmt.Sprintf("Benchmark %d", count),
			}

			payloadBytes, _ := json.Marshal(payload)
			task := asynq.NewTask("benchmark:task", payloadBytes)

			if _, err := client.Enqueue(task); err != nil {
				log.Printf("Failed to queue task: %v", err)
			}

			if count%1000 == 0 {
				fmt.Printf("   Queued: %d tasks\n", count)
			}

			// Small delay to avoid overwhelming the queue
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Monitor progress
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			results := stats.GetResults()
			fmt.Printf("⏱️  Progress: %d tasks processed | %.2f tasks/sec\n",
				results["tasks_processed"],
				results["tasks_per_second"])
		}
	}()

	// Wait for test duration
	fmt.Printf("⏳ Running benchmark for %d seconds...\n\n", config.TestDurationSec)
	time.Sleep(time.Duration(config.TestDurationSec) * time.Second)

	// Shutdown worker
	fmt.Println("\n🛑 Stopping worker...")
	srv.Shutdown()

	// Get final results
	results := stats.GetResults()
	results["worker_count"] = config.WorkerCount

	// Save results to JSON
	filename := "queue_benchmark_results.json"
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal results: %v", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		log.Fatalf("Failed to write results: %v", err)
	}

	// Display results
	fmt.Println("\n✅ BENCHMARK COMPLETE!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Tasks processed: %d\n", results["tasks_processed"])
	fmt.Printf("Tasks failed: %d\n", results["tasks_failed"])
	fmt.Printf("Tasks per second: %.2f\n", results["tasks_per_second"])
	fmt.Printf("Min duration: %dms\n", results["min_duration_ms"])
	fmt.Printf("Max duration: %dms\n", results["max_duration_ms"])
	fmt.Printf("Avg duration: %.2fms\n", results["avg_duration_ms"])
	fmt.Printf("Worker count: %d\n", results["worker_count"])
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("\n💾 Results saved to: %s\n", filename)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}
