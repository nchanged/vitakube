use anyhow::Result;
use tracing::{info, warn};
use std::env;
use std::time::Duration;

mod system_metrics;
mod container_metrics;
mod pvc_metrics;
mod metrics_sender;

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize logging
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info"))
        )
        .with_target(false)
        .compact()
        .init();

    // Get node name from environment (set by Kubernetes)
    let node_name = env::var("NODE_NAME").unwrap_or_else(|_| "unknown".to_string());

    // Get consumer endpoint
    let consumer_endpoint = env::var("CONSUMER_ENDPOINT")
        .unwrap_or_else(|_| "http://vita-consumer:8080/api/v1/ingest".to_string());

    // Get collection interval (default: 1 second for high-frequency monitoring)
    let interval_secs = env::var("COLLECTION_INTERVAL")
        .ok()
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(1);

    info!("🚀 VitaAgent starting | node={} interval={}s endpoint={}", 
          node_name, interval_secs, consumer_endpoint);

    // Initialize metrics sender
    let mut sender = metrics_sender::MetricsSender::new(consumer_endpoint, node_name.clone());

    // Main collection loop
    loop {
        // Collect system-wide metrics from /proc and /sys
        match system_metrics::collect_system_metrics(&node_name, &mut sender) {
            Ok(_) => {},
            Err(e) => warn!("⚠️  System metrics failed: {}", e),
        }

        // Collect container metrics from cgroups
        match container_metrics::collect_container_metrics(&node_name, &mut sender) {
            Ok(_) => {},
            Err(e) => warn!("⚠️  Container metrics failed: {}", e),
        }

        // Collect PVC metrics
        match pvc_metrics::collect_pvc_metrics(&node_name, &mut sender) {
            Ok(_) => {},
            Err(e) => warn!("⚠️  PVC metrics failed: {}", e),
        }

        // Flush metrics to consumer
        if let Err(e) = sender.flush().await {
            warn!("⚠️  Failed to flush metrics: {}", e);
        }

        // Wait before next collection cycle
        tokio::time::sleep(Duration::from_secs(interval_secs)).await;
    }
}
