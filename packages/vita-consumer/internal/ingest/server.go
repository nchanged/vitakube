package ingest

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/nchanged/vitakube/packages/vita-consumer/internal/buffer"
)

type IDResolver interface {
	GetResourceID(uid, rType string) (int64, bool)
}

type IngestionServer struct {
	buffer   *buffer.RingBuffer
	resolver IDResolver
}

func NewIngestionServer(buf *buffer.RingBuffer, res IDResolver) *IngestionServer {
	return &IngestionServer{
		buffer:   buf,
		resolver: res,
	}
}

type IngestRequest struct {
	NodeName string      `json:"node"`
	Metrics  []RawMetric `json:"metrics"`
}

type RawMetric struct {
	Type        string  `json:"type"`              // "container", "node_cpu", "pvc_usage"
	PodID       string  `json:"pod_id,omitempty"`  // For containers (slice path)
	PodUID      string  `json:"pod_uid,omitempty"` // For PVCs (pod using the volume)
	Volume      string  `json:"volume,omitempty"`  // For PVCs (volume name, may contain pvc UID)
	ContainerID string  `json:"container_id,omitempty"`
	Key         string  `json:"key"` // "cpu_ms", "mem_mb", "total_mb", "used_mb", "free_mb"
	Value       float64 `json:"value"`
	Timestamp   int64   `json:"ts"` // unix epoch
}

var podSliceRegex = regexp.MustCompile(`pod([0-9a-fA-F_]+)(?:\.slice)?`)
var pvcVolumeRegex = regexp.MustCompile(`^pvc-([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

func (s *IngestionServer) HandleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	for _, raw := range req.Metrics {
		var resourceID int64
		var uid string
		var rType string = "pod" // default

		// 1. Resolve UID and Type based on metric type
		if raw.Key == "pvc_usage" || strings.Contains(raw.Key, "_mb") && raw.Volume != "" {
			// PVC/Volume metrics
			// First, check if the volume name indicates an actual PVC
			if matches := pvcVolumeRegex.FindStringSubmatch(raw.Volume); len(matches) > 1 {
				// This is an actual PVC - extract PVC UID from volume name
				uid = matches[1]
				rType = "pvc"
			} else {
				// Non-PVC volume (configmap, secret, emptydir, etc.)
				// Link to the pod consuming it
				if raw.PodUID != "" {
					uid = raw.PodUID
					rType = "pod"
				}
			}
		} else {
			// Container metrics
			if raw.PodID != "" {
				matches := podSliceRegex.FindStringSubmatch(raw.PodID)
				if len(matches) > 1 {
					uid = strings.ReplaceAll(matches[1], "_", "-")
					rType = "pod"
				}
			}
		}

		// 2. Resolve DB ID
		if uid != "" {
			if id, ok := s.resolver.GetResourceID(uid, rType); ok {
				resourceID = id
			}
		}

		m := buffer.Metric{
			Time:       time.Unix(raw.Timestamp, 0),
			ResourceID: resourceID,
			Type:       raw.Key,
			Value:      raw.Value,
		}
		s.buffer.Add(m)
	}

	w.WriteHeader(http.StatusAccepted)
}
