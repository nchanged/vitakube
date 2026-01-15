use anyhow::Result;
use std::fs;
use std::path::Path;
use tracing::{info, warn};

pub fn collect_container_metrics(node_name: &str, _sender: &mut crate::metrics_sender::MetricsSender) -> Result<()> {
    // Try to detect cgroup version
    let cgroup_v2 = Path::new("/sys/fs/cgroup/cgroup.controllers").exists();
    
    if cgroup_v2 {
        info!("Generations: Cgroup v2 detected");
        collect_cgroup_v2_metrics(node_name)?;
    } else {
        // info!("Generations: Cgroup v1 detected");
        collect_cgroup_v1_metrics(node_name)?;
    }

    Ok(())
}

fn collect_cgroup_v2_metrics(node_name: &str) -> Result<()> {
    let base_path = Path::new("/sys/fs/cgroup");
    
    // Find pod cgroups
    let kubepods = base_path.join("kubepods.slice");
    if !kubepods.exists() {
        // ... debug logs ...
    }

    if let Ok(entries) = fs::read_dir(&kubepods) {
        for entry in entries.flatten() {
            let path = entry.path();
            if path.is_dir() {
                if let Some(name) = path.file_name().and_then(|n| n.to_str()) {
                    if name.starts_with("kubepods-") {
                        collect_pod_cgroup_v2(&path, name, node_name)?;
                    }
                }
            }
        }
    }

    Ok(())
}

fn collect_pod_cgroup_v2(path: &Path, name: &str, node_name: &str) -> Result<()> {
    let mut cpu_ms = 0u64;
    let mut mem_mb = 0u64;
    let mut mem_limit_mb = 0u64;
    
    // Read CPU stats
    if let Ok(cpu_stat) = fs::read_to_string(path.join("cpu.stat")) {
        for line in cpu_stat.lines() {
            if line.starts_with("usage_usec") {
                let parts: Vec<&str> = line.split_whitespace().collect();
                if parts.len() == 2 {
                    if let Ok(usec) = parts[1].parse::<u64>() {
                        cpu_ms = usec / 1000;
                    }
                }
            }
        }
    }
    
    // Read memory stats
    if let Ok(mem_current) = fs::read_to_string(path.join("memory.current")) {
        if let Ok(bytes) = mem_current.trim().parse::<u64>() {
            mem_mb = bytes / 1024 / 1024;
        }
    }
    
    if let Ok(mem_max) = fs::read_to_string(path.join("memory.max")) {
        if mem_max.trim() != "max" {
            if let Ok(bytes) = mem_max.trim().parse::<u64>() {
                mem_limit_mb = bytes / 1024 / 1024;
            }
        }
    }

    info!("METRIC_TYPE=container node={} pod_id={} cpu_ms={} mem_mb={} mem_limit_mb={}", 
        node_name, name, cpu_ms, mem_mb, mem_limit_mb);

    Ok(())
}

fn collect_cgroup_v1_metrics(node_name: &str) -> Result<()> {
    // Common k8s cgroup v1 paths
    let cpu_base = Path::new("/sys/fs/cgroup/cpu/kubepods");
    let cpu_base_slice = Path::new("/sys/fs/cgroup/cpu/kubepods.slice"); // Systemd driver
    
    let search_path = if cpu_base.exists() {
        cpu_base
    } else if cpu_base_slice.exists() {
        cpu_base_slice
    } else {
        // ... debug ...
        return Ok(());
    };
    
    // Start processing from the base path
    process_v1_dir(search_path, node_name)?;
    Ok(())
}

fn process_v1_dir(dir: &Path, node_name: &str) -> Result<()> {
    match fs::read_dir(dir) {
        Ok(entries) => {
            for entry in entries.flatten() {
                let path = entry.path();
                if path.is_dir() {
                    if let Some(name) = path.file_name().and_then(|n| n.to_str()) {
                        // Prioritize POD detection because pod names might contain qos keywords like 'burstable'
                        if name.starts_with("pod") || name.contains("-pod") {
                            // Found a POD directory
                            process_v1_pod(&path, name, node_name)?;
                        } else if name.contains("burstable") || name.contains("besteffort") || name.contains("guaranteed") {
                            // Recurse into QoS slices
                            process_v1_dir(&path, node_name)?;
                        } 
                    }
                }
            }
        },
        Err(e) => {
           // Only warn if it's not a common 'not found' issues or if we expected it to work
           if dir.to_string_lossy().contains("kubepods") {
               warn!("Failed to read dir {:?}: {}", dir, e);
           }
        }
    }
    Ok(())
}

fn process_v1_pod(pod_path: &Path, pod_name: &str, node_name: &str) -> Result<()> {
    let mut found_container = false;
    match fs::read_dir(pod_path) {
        Ok(entries) => {
            for entry in entries.flatten() {
                let path = entry.path();
                if path.is_dir() {
                    if let Some(name) = path.file_name().and_then(|n| n.to_str()) {
                        let is_container = name.len() > 20 || name.starts_with("docker-") || name.starts_with("crio-");
                        
                        if is_container {
                            // info!("Found container candidate: {}", name);
                            collect_container_cgroup_v1(&path, pod_name, name, node_name)?;
                            found_container = true;
                        }
                    }
                }
            }
            
            if !found_container {
                 info!("Debug: No containers found in pod {}. Contents:", pod_name);
                 if let Ok(debug_entries) = fs::read_dir(pod_path) {
                     for e in debug_entries.flatten() {
                         if let Some(n) = e.file_name().to_str() {
                             info!("  - {}", n);
                         }
                     }
                 }
            }
        },
        Err(e) => {
            warn!("Failed to read pod dir {:?}: {}", pod_path, e);
        }
    }
    Ok(())
}

fn collect_container_cgroup_v1(cpu_path: &Path, pod_id: &str, container_id: &str, node_name: &str) -> Result<()> {
    let mut cpu_ms = 0u64;
    let mut mem_mb = 0u64;
    let mut mem_limit_mb = 0u64;
    
    // Read CPU usage
    if let Ok(cpu_usage) = fs::read_to_string(cpu_path.join("cpuacct.usage")) {
        if let Ok(nanosecs) = cpu_usage.trim().parse::<u64>() {
            cpu_ms = nanosecs / 1_000_000;
        }
    }
    
    // Read memory from corresponding memory cgroup
    let mem_path = cpu_path.to_string_lossy().replace("/cpu/", "/memory/");
    let mem_path = Path::new(&mem_path);
    
    if let Ok(mem_usage) = fs::read_to_string(mem_path.join("memory.usage_in_bytes")) {
        if let Ok(bytes) = mem_usage.trim().parse::<u64>() {
            mem_mb = bytes / 1024 / 1024;
        }
    }
    
    if let Ok(mem_limit) = fs::read_to_string(mem_path.join("memory.limit_in_bytes")) {
        if let Ok(bytes) = mem_limit.trim().parse::<u64>() {
            if bytes < u64::MAX / 2 {
                mem_limit_mb = bytes / 1024 / 1024;
            }
        }
    }

    info!("METRIC_TYPE=container node={} pod_id={} container_id={} cpu_ms={} mem_mb={} mem_limit_mb={}", 
        node_name,
        pod_id,
        container_id,
        cpu_ms, mem_mb, mem_limit_mb);

    Ok(())
}
