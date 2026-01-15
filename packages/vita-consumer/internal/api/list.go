package api

import (
	"database/sql"
	"net/http"
)

// Node represents a cluster node
type Node struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	UID  string `json:"uid"`
}

// Namespace represents a K8s namespace
type Namespace struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Deployment represents a K8s deployment
type Deployment struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	UID         string `json:"uid"`
	NamespaceID int64  `json:"namespace_id"`
	Namespace   string `json:"namespace"`
}

// Pod represents a K8s pod
type Pod struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	UID          string  `json:"uid"`
	NamespaceID  int64   `json:"namespace_id"`
	Namespace    string  `json:"namespace"`
	NodeID       int64   `json:"node_id"`
	NodeName     string  `json:"node"`
	DeploymentID *int64  `json:"deployment_id,omitempty"`
	Deployment   *string `json:"deployment,omitempty"`
}

// PVC represents a PersistentVolumeClaim
type PVC struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	UID         string `json:"uid"`
	NamespaceID int64  `json:"namespace_id"`
	Namespace   string `json:"namespace"`
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := s.sqlite.Query("SELECT id, name, uid FROM nodes ORDER BY name")
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	nodes := []Node{}
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Name, &n.UID); err != nil {
			continue
		}
		nodes = append(nodes, n)
	}

	writeJSON(w, nodes)
}

func (s *Server) handleListNamespaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := s.sqlite.Query("SELECT id, name FROM namespaces ORDER BY name")
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	namespaces := []Namespace{}
	for rows.Next() {
		var ns Namespace
		if err := rows.Scan(&ns.ID, &ns.Name); err != nil {
			continue
		}
		namespaces = append(namespaces, ns)
	}

	writeJSON(w, namespaces)
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := `
		SELECT d.id, d.name, d.uid, d.namespace_id, n.name
		FROM deployments d
		JOIN namespaces n ON d.namespace_id = n.id
	`
	args := []interface{}{}

	if nsID, ok := getQueryInt(r, "namespace"); ok {
		query += " WHERE d.namespace_id = ?"
		args = append(args, nsID)
	}

	query += " ORDER BY d.name"

	rows, err := s.sqlite.Query(query, args...)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	deployments := []Deployment{}
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(&d.ID, &d.Name, &d.UID, &d.NamespaceID, &d.Namespace); err != nil {
			continue
		}
		deployments = append(deployments, d)
	}

	writeJSON(w, deployments)
}

func (s *Server) handleListPods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := `
		SELECT p.id, p.name, p.uid, p.namespace_id, ns.name, p.node_id, n.name, p.deployment_id, d.name
		FROM pods p
		JOIN namespaces ns ON p.namespace_id = ns.id
		JOIN nodes n ON p.node_id = n.id
		LEFT JOIN deployments d ON p.deployment_id = d.id
		WHERE 1=1
	`
	args := []interface{}{}

	if depID, ok := getQueryInt(r, "deployment"); ok {
		query += " AND p.deployment_id = ?"
		args = append(args, depID)
	}
	if nsID, ok := getQueryInt(r, "namespace"); ok {
		query += " AND p.namespace_id = ?"
		args = append(args, nsID)
	}
	if nodeID, ok := getQueryInt(r, "node"); ok {
		query += " AND p.node_id = ?"
		args = append(args, nodeID)
	}

	query += " ORDER BY p.name"

	rows, err := s.sqlite.Query(query, args...)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	pods := []Pod{}
	for rows.Next() {
		var p Pod
		var depName sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.UID, &p.NamespaceID, &p.Namespace, &p.NodeID, &p.NodeName, &p.DeploymentID, &depName); err != nil {
			continue
		}
		if depName.Valid {
			p.Deployment = &depName.String
		}
		pods = append(pods, p)
	}

	writeJSON(w, pods)
}

func (s *Server) handleListPVCs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := `
		SELECT pvc.id, pvc.name, pvc.uid, pvc.namespace_id, n.name
		FROM pvcs pvc
		JOIN namespaces n ON pvc.namespace_id = n.id
	`
	args := []interface{}{}

	if nsID, ok := getQueryInt(r, "namespace"); ok {
		query += " WHERE pvc.namespace_id = ?"
		args = append(args, nsID)
	}

	query += " ORDER BY pvc.name"

	rows, err := s.sqlite.Query(query, args...)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	pvcs := []PVC{}
	for rows.Next() {
		var pvc PVC
		if err := rows.Scan(&pvc.ID, &pvc.Name, &pvc.UID, &pvc.NamespaceID, &pvc.Namespace); err != nil {
			continue
		}
		pvcs = append(pvcs, pvc)
	}

	writeJSON(w, pvcs)
}
