use anyhow::Result;
use std::fs;
use std::path::Path;
use tracing::{info, warn};
use std::ffi::CString;

pub fn collect_pvc_metrics(node_name: &str, _sender: &mut crate::metrics_sender::MetricsSender) -> Result<()> {
    let pods_dir = Path::new("/var/lib/kubelet/pods");
    if !pods_dir.exists() {
        // debug!("PVC Metrics: /var/lib/kubelet/pods does not exist");
        return Ok(());
    }

    if let Ok(entries) = fs::read_dir(pods_dir) {
        for entry in entries.flatten() {
            let path = entry.path();
            if path.is_dir() {
                if let Some(pod_uid) = path.file_name().and_then(|n| n.to_str()) {
                    process_pod_volumes(&path, pod_uid, node_name)?;
                }
            }
        }
    }
    Ok(())
}

fn process_pod_volumes(pod_path: &Path, pod_uid: &str, node_name: &str) -> Result<()> {
    // Structure: /var/lib/kubelet/pods/<UID>/volumes/<DRIVER>/<VOL_NAME>
    // e.g. .../volumes/kubernetes.io~csi/pvc-123.../mount
    // e.g. .../volumes/kubernetes.io~empty-dir/logs
    
    let volumes_path = pod_path.join("volumes");
    if !volumes_path.exists() {
        return Ok(());
    }

    if let Ok(drivers) = fs::read_dir(volumes_path) {
        for driver_entry in drivers.flatten() {
            let driver_path = driver_entry.path();
            if driver_path.is_dir() {
                if let Ok(volumes) = fs::read_dir(&driver_path) {
                    for vol_entry in volumes.flatten() {
                        let vol_path = vol_entry.path();
                        if vol_path.is_dir() {
                            if let Some(vol_name) = vol_path.file_name().and_then(|n| n.to_str()) {
                                // For CSI volumes, the mountpoint is usually 'mount' subdir?
                                // Let's try to stat the volume root first, or look for subdirs.
                                // Actually, for PVCs (csi), it's often `<vol_name>/mount`.
                                // For empty-dir it's just `<vol_name>`.
                                
                                // Let's try to find a mountpoint.
                                // If 'mount' exists, use it. Else use vol_path.
                                let mount_point = if vol_path.join("mount").exists() {
                                    vol_path.join("mount")
                                } else {
                                    vol_path.clone()
                                };
                                
                                collect_volume_stats(&mount_point, pod_uid, vol_name, node_name)?;
                            }
                        }
                    }
                }
            }
        }
    }
    Ok(())
}

fn collect_volume_stats(path: &Path, pod_uid: &str, vol_name: &str, node_name: &str) -> Result<()> {
    let path_str = path.to_string_lossy();
    let c_path = CString::new(path_str.as_bytes()).unwrap_or_default();
    
    unsafe {
        let mut stat: libc::statvfs = std::mem::zeroed();
        if libc::statvfs(c_path.as_ptr(), &mut stat) == 0 {
            let block_size = stat.f_frsize as u64; // fundamental filesystem block size
            let total_blocks = stat.f_blocks as u64;
            let free_blocks = stat.f_bavail as u64; // free blocks for unprivileged users
            
            let total_bytes = total_blocks * block_size;
            let free_bytes = free_blocks * block_size;
            let used_bytes = total_bytes.saturating_sub(free_bytes);
            
            let total_mb = total_bytes / 1024 / 1024;
            let used_mb = used_bytes / 1024 / 1024;
            let free_mb = free_bytes / 1024 / 1024;

            // Only log if meaningful size (>1MB) to avoid noise from empty dirs or proc mounts
            if total_mb > 0 {
                 info!("METRIC_TYPE=pvc_usage node={} pod_uid={} volume={} total_mb={} used_mb={} free_mb={}", 
                    node_name, pod_uid, vol_name, total_mb, used_mb, free_mb);
            }
        }
    }
    
    Ok(())
}
