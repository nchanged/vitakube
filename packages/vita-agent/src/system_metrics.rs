use anyhow::Result;
use std::fs;
use tracing::info;

pub fn collect_system_metrics(node_name: &str, _sender: &mut crate::metrics_sender::MetricsSender) -> Result<()> {
    collect_cpu_metrics(node_name)?;
    collect_memory_metrics(node_name)?;
    collect_disk_metrics(node_name)?;
    collect_network_metrics(node_name)?;

    Ok(())
}

fn collect_cpu_metrics(node_name: &str) -> Result<()> {
    // Manually parse /proc/stat
    let content = fs::read_to_string("/proc/stat")?;
    for line in content.lines() {
        if line.starts_with("cpu ") {
            let parts: Vec<&str> = line.split_whitespace().collect();
            // cpu user nice system idle iowait irq softirq steal guest guest_nice
            if parts.len() >= 5 {
                let user: u64 = parts[1].parse().unwrap_or(0);
                let system: u64 = parts[3].parse().unwrap_or(0);
                let idle: u64 = parts[4].parse().unwrap_or(0);
                let iowait: u64 = parts.get(5).and_then(|s| s.parse().ok()).unwrap_or(0);
                
                info!("METRIC_TYPE=node_cpu node={} user={} sys={} idle={} iowait={}", 
                    node_name, user, system, idle, iowait);
            }
            break;
        }
    }
    Ok(())
}

fn collect_memory_metrics(node_name: &str) -> Result<()> {
    let content = fs::read_to_string("/proc/meminfo")?;
    let mut total = 0;
    let mut free = 0;
    let mut available = 0;
    let mut swap_total = 0;
    let mut swap_free = 0;

    for line in content.lines() {
        let parts: Vec<&str> = line.split_whitespace().collect();
        if parts.len() >= 2 {
            let value: u64 = parts[1].parse().unwrap_or(0);
            match parts[0] {
                "MemTotal:" => total = value,
                "MemFree:" => free = value,
                "MemAvailable:" => available = value,
                "SwapTotal:" => swap_total = value,
                "SwapFree:" => swap_free = value,
                _ => {}
            }
        }
    }
    
    let used = total.saturating_sub(free);
    info!("METRIC_TYPE=node_mem node={} total_mb={} used_mb={} free_mb={} avail_mb={}", 
        node_name, total / 1024, used / 1024, free / 1024, available / 1024);

    if swap_total > 0 {
        let swap_used = swap_total.saturating_sub(swap_free);
        info!("METRIC_TYPE=node_swap node={} total_mb={} used_mb={}", 
            node_name, swap_total / 1024, swap_used / 1024);
    }

    Ok(())
}

fn collect_disk_metrics(node_name: &str) -> Result<()> {
    if let Ok(content) = fs::read_to_string("/proc/diskstats") {
        for line in content.lines() {
            let parts: Vec<&str> = line.split_whitespace().collect();
            // major minor name reads_success reads_merged sectors_read time_read writes_success ...
            if parts.len() >= 14 {
                let name = parts[2];
                if name.starts_with("loop") || name.starts_with("ram") { continue; }
                
                let reads: u64 = parts[3].parse().unwrap_or(0);
                let sectors_read: u64 = parts[5].parse().unwrap_or(0);
                let writes: u64 = parts[7].parse().unwrap_or(0);
                let sectors_written: u64 = parts[9].parse().unwrap_or(0);

                if reads > 0 || writes > 0 {
                    info!("METRIC_TYPE=node_disk node={} device={} reads={} writes={} sectors_r={} sectors_w={}", 
                        node_name, name, reads, writes, sectors_read, sectors_written);
                }
            }
        }
    }
    Ok(())
}

fn collect_network_metrics(node_name: &str) -> Result<()> {
    // Manually parse /proc/net/dev
    // Skip header lines
    if let Ok(content) = fs::read_to_string("/proc/net/dev") {
        for line in content.lines().skip(2) {
            let parts: Vec<&str> = line.split_whitespace().collect();
            if parts.len() >= 17 {
                let name = parts[0].trim_end_matches(':');
                // Skip loopback and veth interfaces to reduce noise
                if name == "lo" || name.starts_with("veth") { continue; }
                
                // rx_bytes packets errs drop fifo frame compressed multicast | tx_bytes packets ...
                let rx_bytes: u64 = parts[1].parse().unwrap_or(0);
                let rx_packets: u64 = parts[2].parse().unwrap_or(0);
                let rx_errs: u64 = parts[3].parse().unwrap_or(0);
                
                let tx_bytes: u64 = parts[9].parse().unwrap_or(0);
                let tx_packets: u64 = parts[10].parse().unwrap_or(0);
                let tx_errs: u64 = parts[11].parse().unwrap_or(0);

                if rx_bytes > 0 || tx_bytes > 0 {
                    info!("METRIC_TYPE=node_net node={} interface={} rx_bytes={} tx_bytes={} rx_pkts={} tx_pkts={} rx_errs={} tx_errs={}", 
                        node_name, name, rx_bytes, tx_bytes, rx_packets, tx_packets, rx_errs, tx_errs);
                }
            }
        }
    }
    Ok(())
}
