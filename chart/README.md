# VitaKube Metrics Agent Helm Chart

This Helm chart deploys the VitaKube Metrics Agent as a **DaemonSet** to your Kubernetes cluster, ensuring one agent pod runs on every node.

## How It Works

The agent collects **real-time metrics** directly from each node's filesystem:
- Reads `/proc` for CPU, memory, load, network stats
- Reads `/sys` for system and device information  
- Reads cgroups for per-container CPU and memory usage

This approach is **ultra-lightweight** and doesn't burden the Kubernetes API server.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+

## Installation

### Install from local chart

```bash
# From the repository root
helm install vita-agent ./chart
```

### Install to a specific namespace

```bash
helm install vita-agent ./chart --namespace monitoring --create-namespace
```

## Configuration

The following table lists the configurable parameters and their default values:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Image repository | `vita-agent` |
| `image.tag` | Image tag | `0.1.0` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.name` | Service account name | `vita-agent` |
| `resources.limits.cpu` | CPU limit | `200m` |
| `resources.limits.memory` | Memory limit | `256Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |
| `collectionInterval` | Metrics collection interval (seconds) | `30` |
| `logLevel` | Logging level | `info` |

### Example: Custom Values

Create a `custom-values.yaml` file:

```yaml
replicaCount: 1

image:
  repository: myregistry.io/vita-agent
  tag: "0.1.0"

resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 200m
    memory: 256Mi

collectionInterval: 60
logLevel: debug
```

Install with custom values:

```bash
helm install vita-agent ./chart -f custom-values.yaml
```

## RBAC Permissions

The chart creates the following RBAC resources:

- **ServiceAccount**: `vita-agent`
- **ClusterRole**: read-only access to:
  - Nodes and node status
  - Pods and pod status (all namespaces)
  - Metrics API (if metrics-server is installed)
- **ClusterRoleBinding**: binds the ServiceAccount to the ClusterRole

## Upgrading

```bash
helm upgrade vita-agent ./chart
```

## Uninstalling

```bash
helm uninstall vita-agent
```

## Viewing Logs

To view the agent logs:

```bash
kubectl logs -f deployment/vita-agent-vita-agent
```

Or if installed in a specific namespace:

```bash
kubectl logs -f deployment/vita-agent-vita-agent -n monitoring
```

## Troubleshooting

### Agent not collecting metrics

1. Check if the pod is running:
   ```bash
   kubectl get pods -l app.kubernetes.io/name=vita-agent
   ```

2. Check the logs:
   ```bash
   kubectl logs -l app.kubernetes.io/name=vita-agent
   ```

3. Verify RBAC permissions:
   ```bash
   kubectl auth can-i list nodes --as=system:serviceaccount:default:vita-agent
   kubectl auth can-i list pods --as=system:serviceaccount:default:vita-agent
   ```

### Permission denied errors

Ensure the ClusterRole and ClusterRoleBinding are created:

```bash
kubectl get clusterrole vita-agent-vita-agent
kubectl get clusterrolebinding vita-agent-vita-agent
```

## Future Enhancements

- Prometheus metrics endpoint
- Push-based metrics delivery  
- Configurable metric filters
- Custom metric aggregations
