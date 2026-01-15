package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nchanged/vitakube/packages/vita-consumer/internal/api"
	"github.com/nchanged/vitakube/packages/vita-consumer/internal/buffer"
	"github.com/nchanged/vitakube/packages/vita-consumer/internal/ingest"
	"github.com/nchanged/vitakube/packages/vita-consumer/internal/store"
	"github.com/nchanged/vitakube/packages/vita-consumer/internal/syncer"
)

func main() {
	// 0. Configuration
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = ".data"
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data dir: %v", err)
	}
	log.Printf("Using data directory: %s", dataDir)

	// 1. Initialize Stores
	sqlite, err := store.NewSQLiteStore(filepath.Join(dataDir, "meta.db"))
	if err != nil {
		log.Fatalf("Failed to open SQLite: %v", err)
	}
	defer sqlite.Close()

	duck, err := store.NewDuckDBStore(filepath.Join(dataDir, "metrics.duckdb"))
	if err != nil {
		log.Fatalf("Failed to open DuckDB: %v", err)
	}
	defer duck.Close()

	// 2. Initialize Syncer
	kubeConfig := os.Getenv("KUBECONFIG")
	if kubeConfig == "" {
		kubeConfig = os.ExpandEnv("$HOME/.kube/config")
	}

	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		kubeConfig = "" // Force in-cluster config
	}

	sync, err := syncer.NewResourceSyncer(kubeConfig, sqlite)
	if err != nil {
		log.Fatalf("Failed to create Syncer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sync.Start(ctx)

	// 3. Initialize Buffer
	ring := buffer.NewRingBuffer(10000) // Hold 10k metrics in RAM

	// 4. Ingestion Server
	ingestion := ingest.NewIngestionServer(ring, sync)
	http.HandleFunc("/api/v1/ingest", ingestion.HandleIngest)

	// 5. API Server (Dashboard Endpoints)
	apiServer := api.NewServer(sqlite, ring)
	apiServer.RegisterRoutes(http.DefaultServeMux)

	// 5. Persist Worker (The Cold Path)
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				data := ring.Flush()
				if len(data) > 0 {
					log.Printf("Flushing %d metrics to DuckDB...", len(data))

					points := make([]store.MetricPoint, len(data))
					for i, m := range data {
						points[i] = store.MetricPoint{
							Time:       m.Time,
							ResourceID: m.ResourceID,
							MetricType: m.Type,
							Value:      m.Value,
						}
					}

					if err := duck.BatchInsert(points); err != nil {
						log.Printf("Error flushing to DuckDB: %v", err)
					}
				}
			}
		}
	}()

	// 6. Start HTTP Server
	go func() {
		log.Println("Starting Consumer on :8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatalf("HTTP Server failed: %v", err)
		}
	}()

	// Wait for signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down...")
}
