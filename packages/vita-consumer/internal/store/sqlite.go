package store

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	// Enable FK
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, err
	}

	if err := initSchema(db); err != nil {
		return nil, err
	}

	return &SQLiteStore{db: db}, nil
}

func initSchema(db *sql.DB) error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS namespaces (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT UNIQUE NOT NULL
        );`,
		`CREATE TABLE IF NOT EXISTS nodes (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            uid TEXT UNIQUE NOT NULL,
            name TEXT NOT NULL,
            updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
        );`,
		// Controllers
		`CREATE TABLE IF NOT EXISTS deployments (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            uid TEXT UNIQUE NOT NULL,
            name TEXT NOT NULL,
            namespace_id INTEGER NOT NULL,
            updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY(namespace_id) REFERENCES namespaces(id)
        );`,
		`CREATE TABLE IF NOT EXISTS statefulsets (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            uid TEXT UNIQUE NOT NULL,
            name TEXT NOT NULL,
            namespace_id INTEGER NOT NULL,
            updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY(namespace_id) REFERENCES namespaces(id)
        );`,
		`CREATE TABLE IF NOT EXISTS daemonsets (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            uid TEXT UNIQUE NOT NULL,
            name TEXT NOT NULL,
            namespace_id INTEGER NOT NULL,
            updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY(namespace_id) REFERENCES namespaces(id)
        );`,
		// Pods
		`CREATE TABLE IF NOT EXISTS pods (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            uid TEXT UNIQUE NOT NULL,
            name TEXT NOT NULL,
            namespace_id INTEGER NOT NULL,
            node_id INTEGER NOT NULL,
            
            -- Linkages
            deployment_id INTEGER,
            statefulset_id INTEGER,
            daemonset_id INTEGER,
            
            updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY(namespace_id) REFERENCES namespaces(id),
            FOREIGN KEY(node_id) REFERENCES nodes(id),
            FOREIGN KEY(deployment_id) REFERENCES deployments(id),
            FOREIGN KEY(statefulset_id) REFERENCES statefulsets(id),
            FOREIGN KEY(daemonset_id) REFERENCES daemonsets(id)
        );`,
		// PVCs
		`CREATE TABLE IF NOT EXISTS pvcs (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            uid TEXT UNIQUE NOT NULL,
            name TEXT NOT NULL,
            namespace_id INTEGER NOT NULL,
            
            -- Can contain NULL if not bound yet? Or Agent only sends bounds?
            -- Syncer will see them.
            
            -- Linkages
            -- Only logic link to pod via volume reference in pod spec? 
            -- Or pod using pvc?
            -- For now, generic link is unused in SQL unless we parse Pod Volumes.
            -- But user asked for table.
            
            updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY(namespace_id) REFERENCES namespaces(id)
        );`,

		// Indexes
		`CREATE INDEX IF NOT EXISTS idx_pods_uid ON pods(uid);`,
		`CREATE INDEX IF NOT EXISTS idx_pvcs_uid ON pvcs(uid);`,
	}

	for _, q := range schemas {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- Specific Upserts ---

func (s *SQLiteStore) UpsertNamespace(name string) (int64, error) {
	query := `INSERT INTO namespaces (name) VALUES (?) 
              ON CONFLICT(name) DO UPDATE SET name=name RETURNING id`
	var id int64
	err := s.db.QueryRow(query, name).Scan(&id)
	return id, err
}

func (s *SQLiteStore) UpsertNode(uid, name string) (int64, error) {
	query := `INSERT INTO nodes (uid, name, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
              ON CONFLICT(uid) DO UPDATE SET name=excluded.name, updated_at=CURRENT_TIMESTAMP RETURNING id`
	var id int64
	err := s.db.QueryRow(query, uid, name).Scan(&id)
	return id, err
}

func (s *SQLiteStore) UpsertDeployment(uid, name string, nsID int64) (int64, error) {
	query := `INSERT INTO deployments (uid, name, namespace_id, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
              ON CONFLICT(uid) DO UPDATE SET name=excluded.name, namespace_id=excluded.namespace_id, updated_at=CURRENT_TIMESTAMP RETURNING id`
	var id int64
	err := s.db.QueryRow(query, uid, name, nsID).Scan(&id)
	return id, err
}

func (s *SQLiteStore) UpsertStatefulSet(uid, name string, nsID int64) (int64, error) {
	query := `INSERT INTO statefulsets (uid, name, namespace_id, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
              ON CONFLICT(uid) DO UPDATE SET name=excluded.name, namespace_id=excluded.namespace_id, updated_at=CURRENT_TIMESTAMP RETURNING id`
	var id int64
	err := s.db.QueryRow(query, uid, name, nsID).Scan(&id)
	return id, err
}

func (s *SQLiteStore) UpsertDaemonSet(uid, name string, nsID int64) (int64, error) {
	query := `INSERT INTO daemonsets (uid, name, namespace_id, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
              ON CONFLICT(uid) DO UPDATE SET name=excluded.name, namespace_id=excluded.namespace_id, updated_at=CURRENT_TIMESTAMP RETURNING id`
	var id int64
	err := s.db.QueryRow(query, uid, name, nsID).Scan(&id)
	return id, err
}

func (s *SQLiteStore) UpsertPod(uid, name string, nsID, nodeID int64, depID, stsID, dsID *int64) (int64, error) {
	query := `
    INSERT INTO pods (uid, name, namespace_id, node_id, deployment_id, statefulset_id, daemonset_id, updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
    ON CONFLICT(uid) DO UPDATE SET
        name = excluded.name,
        namespace_id = excluded.namespace_id,
        node_id = excluded.node_id,
        deployment_id = excluded.deployment_id,
        statefulset_id = excluded.statefulset_id,
        daemonset_id = excluded.daemonset_id,
        updated_at = CURRENT_TIMESTAMP
    RETURNING id;
    `
	var id int64
	err := s.db.QueryRow(query, uid, name, nsID, nodeID, depID, stsID, dsID).Scan(&id)
	return id, err
}

func (s *SQLiteStore) UpsertPVC(uid, name string, nsID int64) (int64, error) {
	query := `
    INSERT INTO pvcs (uid, name, namespace_id, updated_at)
    VALUES (?, ?, ?, CURRENT_TIMESTAMP)
    ON CONFLICT(uid) DO UPDATE SET
        name = excluded.name,
        namespace_id = excluded.namespace_id,
        updated_at = CURRENT_TIMESTAMP
    RETURNING id;
    `
	var id int64
	err := s.db.QueryRow(query, uid, name, nsID).Scan(&id)
	return id, err
}

func (s *SQLiteStore) GetResourceID(table, uid string) (int64, error) {
	var id int64
	query := fmt.Sprintf("SELECT id FROM %s WHERE uid = ?", table)
	err := s.db.QueryRow(query, uid).Scan(&id)
	return id, err
}

// Query executes a SQL query and returns rows
func (s *SQLiteStore) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}
