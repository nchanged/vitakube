# VitaKube Metrics Agent

A **mega-lightweight**, high-performance Kubernetes metrics agent written in Rust that collects real-time metrics directly from the node's filesystem.

## Architecture

Unlike traditional metrics agents that rely heavily on the Kubernetes API, VitaAgent reads metrics **directly from the node's filesystem** for maximum performance and minimal overhead:

- **System Metrics**: Read from `/proc` and `/sys` filesystems
- **Container Metrics**: Read from cgroups (supports both v1 and v2)
- **Deployment Model**: DaemonSet (one pod per node)

## Features

- ⚡ **Ultra-Fast**: Direct filesystem reads with zero API overhead for metrics
- 🪶 **Lightweight**: Minimal resource consumption (~100MB RAM, <100m CPU)
- 📊 **Comprehensive**: Collects both node-level and container-level metrics
- 🔧 **Simple**: No complex dependencies or external metric servers required
- 🐳 **Cloud Native**: Designed for Kubernetes deployment via DaemonSet

## What It Collects

### Node-Level Metrics (from `/proc` and `/sys`)
- **CPU**: User, System, Idle, IOWait time (monotonically increasing ticks)
- **Memory**: Total, Used, Free, Available (MB)
- **Disk I/O**: Reads/Writes count and sectors read/written per physical device (ignores partitions/loop devices)
- **Network I/O**: RX/TX bytes, packets, and errors per interface (automatically filters partial `veth` and `lo` interfaces)
- **System Load**: 1, 5, and 15-minute load averages

### Container Metrics (from Cgroups)
- **CPU Usage**: Cumulative CPU time in milliseconds (v1 `cpuacct.usage` / v2 `cpu.stat`)
- **Memory**: Current usage and Limits in MB
- **Pod Association**: Links containers to their Pod IDs automatically
- **Cgroup v1 & v2**: Automatically detects and supports recursive traversal for both cgroup versions (including Systemd slices)

### Volume & PVC Metrics (from `/var/lib/kubelet`)
- **PVC Usage**: Monitors `kubernetes.io~csi` (PVCs), `empty-dir`, `configmap`, and `secret` volumes
- **Capacity**: Total size (MB)
- **Utilization**: Used space (MB) and Free space (MB)
- **Discovery**: Automatically discovers volumes mapped to active Pods on the node

## Building

To build the agent, you need Rust installed. Then run:

```bash
cd packages/vita-agent
cargo build --release
```

The binary will be available at `target/release/vita-agent`.

## Running Locally

To run the agent locally (requires access to `/proc` and `/sys`):

```bash
cd packages/vita-agent
sudo RUST_LOG=info cargo run
```

**Note**: Root access is needed to read cgroup information.

## Building Docker Image

```bash
cd packages/vita-agent
docker build -t vita-agent:0.1.0 .
```

## Deploying to Kubernetes

The agent runs as a **DaemonSet** (one pod per node) and requires privileged access to read host filesystems.

See the [chart README](../../chart/README.md) for deployment instructions.

## Configuration

The agent can be configured via environment variables:

- `NODE_NAME`: Node name (automatically set by Kubernetes)
- `RUST_LOG`: Log level (trace, debug, info, warn, error) - default: `info`
- `COLLECTION_INTERVAL`: Metrics collection interval in seconds - default: `1`

## Output Format

The agent emits metrics to stdout in a parsable `key=value` format designed for easy ingestion:

### Metric Types uses `METRIC_TYPE=<type>` identifier:

- **Node Metrics**:
  ```text
  METRIC_TYPE=node_cpu node=<name> user=... sys=... idle=...
  METRIC_TYPE=node_mem node=<name> total_mb=... used_mb=...
  METRIC_TYPE=node_disk node=<name> device=sda ...
  METRIC_TYPE=node_net node=<name> interface=eth0 ...
  ```

- **Container Metrics**:
  ```text
  METRIC_TYPE=container node=<name> pod_id=<pod_slice> container_id=<scope> cpu_ms=... mem_mb=...
  ```

- **PVC Metrics**:
  ```text
  METRIC_TYPE=pvc_usage node=<name> pod_uid=<uid> volume=<name> total_mb=... used_mb=... free_mb=...
  ```

## Why Direct Filesystem Access?

Traditional metrics collection via Kubernetes API has limitations:
1. **API overhead**: Every metric requires API calls
2. **Delayed updates**: Metrics are cached and may be stale
3. **Resource intensive**: API server load increases with cluster size

By reading directly from `/proc`, `/sys`, and cgroups:
1. **Real-time data**: Instant access to current metrics
2. **No API overhead**: Zero load on Kubernetes API server
3. **Lightweight**: Minimal resource consumption

## License

MIT
