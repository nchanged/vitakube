package syncer

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nchanged/vitakube/packages/vita-consumer/internal/store"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type ResourceSyncer struct {
	client  *kubernetes.Clientset
	sqlite  *store.SQLiteStore
	factory informers.SharedInformerFactory

	mu sync.RWMutex
	// Caches: UID -> ID
	pods map[string]int64
	pvcs map[string]int64

	// Namespace name -> ID
	namespaces map[string]int64
	// Node name -> ID (Node UID is in DB, but sometimes we only know Name)
	// Actually we need Name->ID for Pod.NodeName lookup if we haven't synced Node object yet.
	// If we sync node object, we know UID. But Pod Spec has NodeName string.
	// So we need Name -> ID cache for Nodes.
	nodes map[string]int64
	// ReplicaSet UID -> Deployment ID (for Pod->Deployment resolution)
	replicaSets map[string]int64
}

func NewResourceSyncer(kubeConfigPath string, sqlite *store.SQLiteStore) (*ResourceSyncer, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kube config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &ResourceSyncer{
		client:      clientset,
		sqlite:      sqlite,
		pods:        make(map[string]int64),
		pvcs:        make(map[string]int64),
		namespaces:  make(map[string]int64),
		nodes:       make(map[string]int64),
		replicaSets: make(map[string]int64),
	}, nil
}

func (s *ResourceSyncer) Start(ctx context.Context) {
	s.factory = informers.NewSharedInformerFactory(s.client, 10*time.Minute)

	podInformer := s.factory.Core().V1().Pods().Informer()
	pvcInformer := s.factory.Core().V1().PersistentVolumeClaims().Informer()
	nodeInformer := s.factory.Core().V1().Nodes().Informer()
	depInformer := s.factory.Apps().V1().Deployments().Informer()
	stsInformer := s.factory.Apps().V1().StatefulSets().Informer()
	dsInformer := s.factory.Apps().V1().DaemonSets().Informer()
	rsInformer := s.factory.Apps().V1().ReplicaSets().Informer()

	// Handlers
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc:    s.syncObject,
		UpdateFunc: func(old, new interface{}) { s.syncObject(new) },
	}

	podInformer.AddEventHandler(handler)
	pvcInformer.AddEventHandler(handler)
	nodeInformer.AddEventHandler(handler)
	depInformer.AddEventHandler(handler)
	stsInformer.AddEventHandler(handler)
	dsInformer.AddEventHandler(handler)
	rsInformer.AddEventHandler(handler)

	s.factory.Start(ctx.Done())
	s.factory.WaitForCacheSync(ctx.Done())

	log.Println("Resource Syncer started and synced")
}

func (s *ResourceSyncer) syncObject(obj interface{}) {
	switch o := obj.(type) {
	case *corev1.Node:
		s.syncNode(o)
	case *corev1.Pod:
		s.syncPod(o)
	case *corev1.PersistentVolumeClaim:
		s.syncPVC(o)
	case *appsv1.Deployment:
		s.syncDeployment(o)
	case *appsv1.StatefulSet:
		s.syncStatefulSet(o)
	case *appsv1.DaemonSet:
		s.syncDaemonSet(o)
	case *appsv1.ReplicaSet:
		s.syncReplicaSet(o)
	}
}

// Helpers to get/set cache
func (s *ResourceSyncer) getNamespaceID(name string) int64 {
	s.mu.RLock()
	id, ok := s.namespaces[name]
	s.mu.RUnlock()
	if ok {
		return id
	}

	// Attempt upsert
	id, err := s.sqlite.UpsertNamespace(name)
	if err != nil {
		log.Printf("Failed to upsert namespace %s: %v", name, err)
		return 0
	}

	s.mu.Lock()
	s.namespaces[name] = id
	s.mu.Unlock()
	return id
}

func (s *ResourceSyncer) getNodeID(name, uid string) int64 {
	// If we have just name (from Pod), we check cache.
	// If not in cache, we might insert it with placeholder UID or check if we have it?
	// We should allow Upsert logic to handle "Name Only" if possible?
	// But Node UID is required by DB.
	// If we are syncing a Node Object, we have UID.
	// If we are syncing a Pod, we only have NodeName.
	// We can assume Node object is synced or will be synced.
	// For now, if called from syncNode, we have UID.
	// If called from syncPod, we might not have UID if node not synced yet.
	// Strategy: Only return ID if we know it (from syncNode).
	// If not, maybe inserting a placeholder "unknown-uid-nodeName"? No.
	// Just return 0/nil and let foreign key be NULL? (Schema says NOT NULL for node_id in pods... wait, schema says NOT NULL).
	// So we MUST have a node ID.
	// Solution: Upsert Node on first sight with UID=Name if real UID unknown? That's hacky.
	// Better: Pod usually runs on Node that EXISTS. Syncer should see Node first or soon.
	// We can create a "stub" node with UID="stub-NAME".

	s.mu.RLock()
	id, ok := s.nodes[name]
	s.mu.RUnlock()

	if ok {
		return id
	}

	if uid == "" {
		// From Pod, unknown UID. Try stub?
		uid = "stub-" + name
	}

	id, err := s.sqlite.UpsertNode(uid, name)
	if err != nil {
		log.Printf("Failed to upsert node %s: %v", name, err)
		return 0
	}

	s.mu.Lock()
	s.nodes[name] = id
	s.mu.Unlock()
	return id
}

func (s *ResourceSyncer) syncNode(n *corev1.Node) {
	s.getNodeID(n.Name, string(n.UID))
}

func (s *ResourceSyncer) syncDeployment(d *appsv1.Deployment) {
	nsID := s.getNamespaceID(d.Namespace)
	_, err := s.sqlite.UpsertDeployment(string(d.UID), d.Name, nsID)
	if err != nil {
		log.Printf("Failed to sync deployment %s: %v", d.Name, err)
	}
}

func (s *ResourceSyncer) syncStatefulSet(sts *appsv1.StatefulSet) {
	nsID := s.getNamespaceID(sts.Namespace)
	_, err := s.sqlite.UpsertStatefulSet(string(sts.UID), sts.Name, nsID)
	if err != nil {
		log.Printf("Failed to sync sts %s: %v", sts.Name, err)
	}
}

func (s *ResourceSyncer) syncDaemonSet(ds *appsv1.DaemonSet) {
	nsID := s.getNamespaceID(ds.Namespace)
	_, err := s.sqlite.UpsertDaemonSet(string(ds.UID), ds.Name, nsID)
	if err != nil {
		log.Printf("Failed to sync ds %s: %v", ds.Name, err)
	}
}

func (s *ResourceSyncer) syncReplicaSet(rs *appsv1.ReplicaSet) {
	// We don't store RS in DB, but we cache the RS UID -> Deployment ID mapping
	rsUID := string(rs.UID)

	for _, owner := range rs.OwnerReferences {
		if owner.Kind == "Deployment" {
			// Look up deployment ID
			if depID, err := s.sqlite.GetResourceID("deployments", string(owner.UID)); err == nil {
				s.mu.Lock()
				s.replicaSets[rsUID] = depID
				s.mu.Unlock()
				return
			}
		}
	}
}

func (s *ResourceSyncer) syncPod(pod *corev1.Pod) {
	uid := string(pod.UID)
	nsID := s.getNamespaceID(pod.Namespace)

	if nsID == 0 {
		log.Printf("Failed to sync pod %s: namespace ID is 0", pod.Name)
		return
	}

	var nodeID int64
	if pod.Spec.NodeName != "" {
		nodeID = s.getNodeID(pod.Spec.NodeName, "")
		if nodeID == 0 {
			log.Printf("Failed to sync pod %s: node ID is 0", pod.Name)
			return
		}
	} else {
		// Pod not scheduled yet, skip for now
		return
	}

	var depID, stsID, dsID *int64

	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "StatefulSet" {
			if id, err := s.sqlite.GetResourceID("statefulsets", string(owner.UID)); err == nil {
				stsID = &id
			}
		} else if owner.Kind == "DaemonSet" {
			if id, err := s.sqlite.GetResourceID("daemonsets", string(owner.UID)); err == nil {
				dsID = &id
			}
		} else if owner.Kind == "ReplicaSet" {
			// Check RS cache for deployment link
			s.mu.RLock()
			if id, ok := s.replicaSets[string(owner.UID)]; ok {
				depID = &id
			}
			s.mu.RUnlock()
		}
	}

	id, err := s.sqlite.UpsertPod(uid, pod.Name, nsID, nodeID, depID, stsID, dsID)
	if err != nil {
		log.Printf("Failed to sync pod %s: %v", pod.Name, err)
		return
	}

	s.mu.Lock()
	s.pods[uid] = id
	s.mu.Unlock()
}

func (s *ResourceSyncer) syncPVC(pvc *corev1.PersistentVolumeClaim) {
	uid := string(pvc.UID)
	nsID := s.getNamespaceID(pvc.Namespace)

	id, err := s.sqlite.UpsertPVC(uid, pvc.Name, nsID)
	if err != nil {
		log.Printf("Failed to sync pvc %s: %v", pvc.Name, err)
		return
	}

	s.mu.Lock()
	s.pvcs[uid] = id
	s.mu.Unlock()
}

func (s *ResourceSyncer) GetResourceID(uid, rType string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if rType == "pvc" {
		id, ok := s.pvcs[uid]
		return id, ok
	}
	id, ok := s.pods[uid]
	return id, ok
}
