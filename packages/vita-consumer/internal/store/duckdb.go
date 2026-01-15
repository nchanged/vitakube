package store

import (
	"database/sql"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

type DuckDBStore struct {
	db *sql.DB
}

type MetricPoint struct {
	Time       time.Time
	ResourceID int64
	MetricType string
	Value      float64
}

func NewDuckDBStore(path string) (*DuckDBStore, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, err
	}

	if err := initDuckDBSchema(db); err != nil {
		return nil, err
	}

	return &DuckDBStore{db: db}, nil
}

func initDuckDBSchema(db *sql.DB) error {
	query := `
    CREATE TABLE IF NOT EXISTS metrics (
        time TIMESTAMPTZ NOT NULL,
        resource_id INTEGER NOT NULL,
        metric_type TEXT NOT NULL,
        value DOUBLE NOT NULL,
        agg_type TEXT DEFAULT 'raw'
    );
    `
	_, err := db.Exec(query)
	return err
}

func (s *DuckDBStore) Close() error {
	return s.db.Close()
}

func (s *DuckDBStore) BatchInsert(metrics []MetricPoint) error {
	if len(metrics) == 0 {
		return nil
	}

	// DuckDB appender API is faster, but for now simple batch INSERT is fine
	// Or transaction.
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Prepared statement
	stmt, err := tx.Prepare("INSERT INTO metrics (time, resource_id, metric_type, value, agg_type) VALUES (?, ?, ?, ?, 'raw')")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range metrics {
		_, err := stmt.Exec(m.Time, m.ResourceID, m.MetricType, m.Value)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
