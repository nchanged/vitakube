package api

import (
	"net/http"
	"time"
)

// LiveMetricsResponse represents the response for live metrics
type LiveMetricsResponse struct {
	Timestamp int64     `json:"timestamp"`
	Pods      []LivePod `json:"pods"`
}

// LivePod represents a pod with its live metrics
type LivePod struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	UID        string          `json:"uid"`
	Namespace  string          `json:"namespace"`
	Node       string          `json:"node"`
	Deployment *string         `json:"deployment,omitempty"`
	Containers []ContainerInfo `json:"containers"`
	PVCs       []PVCInfo       `json:"pvcs"`
}

// ContainerInfo represents container metrics
type ContainerInfo struct {
	ID         string  `json:"id"`
	CPUms      float64 `json:"cpu_ms"`
	MemMB      float64 `json:"mem_mb"`
	MemLimitMB float64 `json:"mem_limit_mb"`
}

// PVCInfo represents PVC metrics
type PVCInfo struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	VolumeName string  `json:"volume_name"`
	TotalMB    float64 `json:"total_mb"`
	UsedMB     float64 `json:"used_mb"`
	FreeMB     float64 `json:"free_mb"`
}

func (s *Server) handleLiveMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get recent metrics from ring buffer (last 5 seconds)
	cutoffTime := time.Now().Add(-5 * time.Second)
	allMetrics := s.ring.ReadAll()

	// Build pod ID set from recent metrics
	activePodIDs := make(map[int64]bool)
	for _, m := range allMetrics {
		if m.Time.After(cutoffTime) && m.ResourceID > 0 {
			activePodIDs[m.ResourceID] = true
		}
	}

	if len(activePodIDs) == 0 {
		writeJSON(w, LiveMetricsResponse{
			Timestamp: time.Now().Unix(),
			Pods:      []LivePod{},
		})
		return
	}

	// Build WHERE clause for SQL query based on filters
	whereClause := "WHERE 1=1"
	args := []interface{}{}

	if depID, ok := getQueryInt(r, "deployment"); ok {
		whereClause += " AND p.deployment_id = ?"
		args = append(args, depID)
	}
	if nodeID, ok := getQueryInt(r, "node"); ok {
		whereClause += " AND p.node_id = ?"
		args = append(args, nodeID)
	}
	if podID, ok := getQueryInt(r, "pod"); ok {
		whereClause += " AND p.id = ?"
		args = append(args, podID)
	}

	// Query pod metadata (we'll filter by active IDs in Go)
	query := `
		SELECT p.id, p.name, p.uid, ns.name, n.name, d.name
		FROM pods p
		JOIN namespaces ns ON p.namespace_id = ns.id
		JOIN nodes n ON p.node_id = n.id
		LEFT JOIN deployments d ON p.deployment_id = d.id
		` + whereClause + `
		ORDER BY p.name
	`

	rows, err := s.sqlite.Query(query, args...)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	pods := []LivePod{}
	for rows.Next() {
		var p LivePod
		var depName *string
		if err := rows.Scan(&p.ID, &p.Name, &p.UID, &p.Namespace, &p.Node, &depName); err != nil {
			continue
		}
		p.Deployment = depName

		// Filter by active pod IDs
		if !activePodIDs[p.ID] {
			continue
		}

		// Aggregate container and PVC metrics for this pod
		containerMetrics := make(map[string]*ContainerInfo)
		pvcMetrics := make(map[int64]*PVCInfo)

		for _, m := range allMetrics {
			if m.ResourceID != p.ID || m.Time.Before(cutoffTime) {
				continue
			}

			// Container metrics (cpu_ms, mem_mb, mem_limit_mb)
			switch m.Type {
			case "cpu_ms":
				if _, ok := containerMetrics["default"]; !ok {
					containerMetrics["default"] = &ContainerInfo{ID: "default"}
				}
				containerMetrics["default"].CPUms = m.Value
			case "mem_mb":
				if _, ok := containerMetrics["default"]; !ok {
					containerMetrics["default"] = &ContainerInfo{ID: "default"}
				}
				containerMetrics["default"].MemMB = m.Value
			case "mem_limit_mb":
				if _, ok := containerMetrics["default"]; !ok {
					containerMetrics["default"] = &ContainerInfo{ID: "default"}
				}
				containerMetrics["default"].MemLimitMB = m.Value
			case "total_mb", "used_mb", "free_mb":
				// PVC metrics - resource_id points to PVC or pod
				// We need to identify which PVC this belongs to
				// For now, aggregate under pod
			}
		}

		for _, c := range containerMetrics {
			p.Containers = append(p.Containers, *c)
		}
		for _, pvc := range pvcMetrics {
			p.PVCs = append(p.PVCs, *pvc)
		}

		pods = append(pods, p)
	}

	writeJSON(w, LiveMetricsResponse{
		Timestamp: time.Now().Unix(),
		Pods:      pods,
	})
}
