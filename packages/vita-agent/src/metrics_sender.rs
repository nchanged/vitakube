use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::time::{SystemTime, UNIX_EPOCH};

#[derive(Debug, Serialize, Deserialize)]
pub struct MetricBatch {
    pub node: String,
    pub metrics: Vec<RawMetric>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct RawMetric {
    #[serde(rename = "type")]
    pub metric_type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pod_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pod_uid: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub volume: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub container_id: Option<String>,
    pub key: String,
    pub value: f64,
    pub ts: i64,
}

pub struct MetricsSender {
    client: reqwest::Client,
    endpoint: String,
    node_name: String,
    batch: Vec<RawMetric>,
}

impl MetricsSender {
    pub fn new(endpoint: String, node_name: String) -> Self {
        Self {
            client: reqwest::Client::new(),
            endpoint,
            node_name,
            batch: Vec::with_capacity(100),
        }
    }

    pub fn add_metric(&mut self, metric: RawMetric) {
        self.batch.push(metric);
    }

    pub async fn flush(&mut self) -> Result<()> {
        if self.batch.is_empty() {
            return Ok(());
        }

        let payload = MetricBatch {
            node: self.node_name.clone(),
            metrics: std::mem::replace(&mut self.batch, Vec::with_capacity(100)),
        };

        match self.client
            .post(&self.endpoint)
            .json(&payload)
            .send()
            .await
        {
            Ok(resp) => {
                if !resp.status().is_success() {
                    tracing::warn!("Failed to send metrics: HTTP {}", resp.status());
                }
            }
            Err(e) => {
                tracing::warn!("Failed to send metrics: {}", e);
            }
        }

        Ok(())
    }
}

pub fn get_timestamp() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64
}
