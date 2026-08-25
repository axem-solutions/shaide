package dashboards

// clusterNodesDashboard deliberately keeps three separate node-identity variables,
// one per exporter's own label domain, since none of them can be joined by regex:
// "node" (node-exporter's instance label, an IP:port) for host-level OS metrics,
// "k8s_node" (kube-state-metrics' node label, the Kubernetes Node name) for
// pod-to-node panels, and "gpu_host" (DCGM exporter's Hostname label, the node's
// hostname) for GPU panels. The chart's default kubernetes-nodes-cadvisor scrape
// job does not relabel the node name onto cAdvisor container metrics, and DCGM
// exporter here has no Kubernetes pod-mapping configured (see gpu-operator
// values), so panels are scoped to whichever variable actually matches their
// metric's label.
const clusterNodesDashboard = `{
  "uid": "cluster-nodes",
  "title": "Cluster · Node Metrics",
  "tags": ["cluster", "metrics", "nodes"],
  "schemaVersion": 38,
  "time": {"from": "now-1h", "to": "now"},
  "refresh": "30s",
  "templating": {
    "list": [
      {
        "name": "datasource",
        "type": "datasource",
        "query": "prometheus",
        "label": "Data source",
        "refresh": 1,
        "hide": 0
      },
      {
        "name": "node",
        "label": "Node (host)",
        "type": "query",
        "datasource": {"type": "prometheus", "uid": "${datasource}"},
        "query": "label_values(node_cpu_seconds_total, instance)",
        "refresh": 2,
        "sort": 1,
        "multi": true,
        "includeAll": true,
        "allValue": ".*",
        "hide": 0
      },
      {
        "name": "k8s_node",
        "label": "Node (Kubernetes name)",
        "type": "query",
        "datasource": {"type": "prometheus", "uid": "${datasource}"},
        "query": "label_values(kube_pod_info, node)",
        "refresh": 2,
        "sort": 1,
        "multi": true,
        "includeAll": true,
        "allValue": ".*",
        "hide": 0
      },
      {
        "name": "gpu_host",
        "label": "Node (GPU host)",
        "type": "query",
        "datasource": {"type": "prometheus", "uid": "${datasource}"},
        "query": "label_values(DCGM_FI_DEV_GPU_UTIL, Hostname)",
        "refresh": 2,
        "sort": 1,
        "multi": true,
        "includeAll": true,
        "allValue": ".*",
        "hide": 0
      }
    ]
  },
  "panels": [
    {"id": 200, "type": "row", "title": "Overview", "collapsed": false, "panels": [], "gridPos": {"h": 1, "w": 24, "x": 0, "y": 0}},
    {
      "id": 101,
      "type": "stat",
      "title": "Pods on Node",
      "description": "Uses the Kubernetes node name (kube-state-metrics), not the host instance variable above.",
      "gridPos": {"h": 4, "w": 6, "x": 0, "y": 1},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {"defaults": {"unit": "short"}, "overrides": []},
      "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "colorMode": "value", "graphMode": "none"},
      "targets": [
        {"datasource": {"type": "prometheus", "uid": "${datasource}"}, "expr": "count(kube_pod_info{node=~\"$k8s_node\"})", "refId": "A"}
      ]
    },
    {
      "id": 102,
      "type": "stat",
      "title": "CPU Cores",
      "gridPos": {"h": 4, "w": 6, "x": 6, "y": 1},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {"defaults": {"unit": "short"}, "overrides": []},
      "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "colorMode": "value", "graphMode": "none"},
      "targets": [
        {"datasource": {"type": "prometheus", "uid": "${datasource}"}, "expr": "count by (instance) (count by (instance, cpu) (node_cpu_seconds_total{instance=~\"$node\"}))", "legendFormat": "{{instance}}", "refId": "A"}
      ]
    },
    {
      "id": 103,
      "type": "stat",
      "title": "RAM Total",
      "gridPos": {"h": 4, "w": 6, "x": 12, "y": 1},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {"defaults": {"unit": "bytes"}, "overrides": []},
      "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "colorMode": "value", "graphMode": "none"},
      "targets": [
        {"datasource": {"type": "prometheus", "uid": "${datasource}"}, "expr": "node_memory_MemTotal_bytes{instance=~\"$node\"}", "legendFormat": "{{instance}}", "refId": "A"}
      ]
    },
    {
      "id": 104,
      "type": "stat",
      "title": "Node Uptime",
      "gridPos": {"h": 4, "w": 6, "x": 18, "y": 1},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {"defaults": {"unit": "s"}, "overrides": []},
      "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "colorMode": "value", "graphMode": "none"},
      "targets": [
        {"datasource": {"type": "prometheus", "uid": "${datasource}"}, "expr": "time() - node_boot_time_seconds{instance=~\"$node\"}", "legendFormat": "{{instance}}", "refId": "A"}
      ]
    },
    {
      "id": 105,
      "type": "table",
      "title": "List of Pods on Node",
      "gridPos": {"h": 6, "w": 24, "x": 0, "y": 5},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {"defaults": {}, "overrides": []},
      "options": {"showHeader": true},
      "targets": [
        {"datasource": {"type": "prometheus", "uid": "${datasource}"}, "expr": "kube_pod_info{node=~\"$k8s_node\"}", "format": "table", "instant": true, "refId": "A"}
      ]
    },
    {"id": 210, "type": "row", "title": "CPU & Memory", "collapsed": false, "panels": [], "gridPos": {"h": 1, "w": 24, "x": 0, "y": 11}},
    {
      "id": 1,
      "type": "timeseries",
      "title": "CPU Usage by Node",
      "gridPos": {"h": 7, "w": 12, "x": 0, "y": 12},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "percentunit",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "1 - avg by (instance) (rate(node_cpu_seconds_total{mode=\"idle\", instance=~\"$node\"}[5m]))",
          "legendFormat": "{{instance}}",
          "refId": "A"
        }
      ]
    },
    {
      "id": 6,
      "type": "timeseries",
      "title": "CPU Usage by Mode",
      "description": "Where CPU time is going per node — iowait spikes point at a disk bottleneck, steal points at a noisy neighbor on the underlying VM host.",
      "gridPos": {"h": 7, "w": 12, "x": 12, "y": 12},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1, "stacking": {"mode": "normal"}},
          "unit": "short",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "desc"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum by (instance, mode) (rate(node_cpu_seconds_total{mode!=\"idle\", instance=~\"$node\"}[5m]))",
          "legendFormat": "{{instance}} / {{mode}}",
          "refId": "A"
        }
      ]
    },
    {
      "id": 2,
      "type": "timeseries",
      "title": "Memory Usage by Node",
      "gridPos": {"h": 7, "w": 12, "x": 0, "y": 19},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "percentunit",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "1 - (node_memory_MemAvailable_bytes{instance=~\"$node\"} / node_memory_MemTotal_bytes{instance=~\"$node\"})",
          "legendFormat": "{{instance}}",
          "refId": "A"
        }
      ]
    },
    {
      "id": 7,
      "type": "timeseries",
      "title": "Memory Breakdown by Node",
      "description": "Used vs. reclaimable (buffers/cache) vs. free — buffers/cache is reclaimable under pressure, so \"used\" alone overstates risk.",
      "gridPos": {"h": 7, "w": 12, "x": 12, "y": 19},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 20, "lineWidth": 1, "stacking": {"mode": "normal"}},
          "unit": "bytes",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "desc"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "node_memory_MemTotal_bytes{instance=~\"$node\"} - node_memory_MemFree_bytes{instance=~\"$node\"} - node_memory_Buffers_bytes{instance=~\"$node\"} - node_memory_Cached_bytes{instance=~\"$node\"}",
          "legendFormat": "{{instance}} used",
          "refId": "A"
        },
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "node_memory_Buffers_bytes{instance=~\"$node\"} + node_memory_Cached_bytes{instance=~\"$node\"}",
          "legendFormat": "{{instance}} buff/cache",
          "refId": "B"
        },
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "node_memory_MemFree_bytes{instance=~\"$node\"}",
          "legendFormat": "{{instance}} free",
          "refId": "C"
        }
      ]
    },
    {"id": 220, "type": "row", "title": "Disk", "collapsed": false, "panels": [], "gridPos": {"h": 1, "w": 24, "x": 0, "y": 26}},
    {
      "id": 3,
      "type": "timeseries",
      "title": "Disk Usage by Node",
      "gridPos": {"h": 7, "w": 12, "x": 0, "y": 27},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "percentunit",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "1 - (node_filesystem_avail_bytes{instance=~\"$node\", fstype!~\"tmpfs|overlay|squashfs\"} / node_filesystem_size_bytes{instance=~\"$node\", fstype!~\"tmpfs|overlay|squashfs\"})",
          "legendFormat": "{{instance}} {{mountpoint}}",
          "refId": "A"
        }
      ]
    },
    {
      "id": 8,
      "type": "timeseries",
      "title": "Disk I/O Throughput by Node",
      "gridPos": {"h": 7, "w": 12, "x": 12, "y": 27},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "Bps"
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "desc"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum by (instance) (rate(node_disk_read_bytes_total{instance=~\"$node\"}[5m]))",
          "legendFormat": "{{instance}} read",
          "refId": "A"
        },
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum by (instance) (rate(node_disk_written_bytes_total{instance=~\"$node\"}[5m]))",
          "legendFormat": "{{instance}} write",
          "refId": "B"
        }
      ]
    },
    {
      "id": 9,
      "type": "timeseries",
      "title": "Disk IOPS by Node",
      "gridPos": {"h": 7, "w": 12, "x": 0, "y": 34},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "iops",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "desc"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum by (instance) (rate(node_disk_reads_completed_total{instance=~\"$node\"}[5m]))",
          "legendFormat": "{{instance}} reads",
          "refId": "A"
        },
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum by (instance) (rate(node_disk_writes_completed_total{instance=~\"$node\"}[5m]))",
          "legendFormat": "{{instance}} writes",
          "refId": "B"
        }
      ]
    },
    {
      "id": 10,
      "type": "timeseries",
      "title": "Disk I/O Utilization by Node",
      "description": "Fraction of time disks spend actively servicing I/O — a saturation signal distinct from throughput; sustained values near 100% indicate the disk, not the workload, is the bottleneck.",
      "gridPos": {"h": 7, "w": 12, "x": 12, "y": 34},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "percentunit",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "avg by (instance) (rate(node_disk_io_time_seconds_total{instance=~\"$node\"}[5m]))",
          "legendFormat": "{{instance}}",
          "refId": "A"
        }
      ]
    },
    {
      "id": 111,
      "type": "timeseries",
      "title": "FS Inode Usage by Node",
      "description": "Filesystems can run out of inodes (small-file limit) well before they run out of space — a distinct failure mode from Disk Usage above.",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 41},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "percentunit",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "1 - (node_filesystem_files_free{instance=~\"$node\", fstype!~\"tmpfs|overlay|squashfs\"} / node_filesystem_files{instance=~\"$node\", fstype!~\"tmpfs|overlay|squashfs\"})",
          "legendFormat": "{{instance}} {{mountpoint}}",
          "refId": "A"
        }
      ]
    },
    {"id": 230, "type": "row", "title": "Network", "collapsed": false, "panels": [], "gridPos": {"h": 1, "w": 24, "x": 0, "y": 48}},
    {
      "id": 4,
      "type": "timeseries",
      "title": "Network I/O by Node",
      "gridPos": {"h": 7, "w": 12, "x": 0, "y": 49},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "Bps"
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "rate(node_network_receive_bytes_total{instance=~\"$node\", device!~\"lo|veth.*|docker.*|cali.*|flannel.*\"}[5m])",
          "legendFormat": "{{instance}} {{device}} rx",
          "refId": "A"
        },
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "rate(node_network_transmit_bytes_total{instance=~\"$node\", device!~\"lo|veth.*|docker.*|cali.*|flannel.*\"}[5m])",
          "legendFormat": "{{instance}} {{device}} tx",
          "refId": "B"
        }
      ]
    },
    {
      "id": 11,
      "type": "timeseries",
      "title": "Network Errors & Drops by Node",
      "gridPos": {"h": 7, "w": 12, "x": 12, "y": 49},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "color": {"fixedColor": "red", "mode": "fixed"},
          "custom": {"drawStyle": "bars", "fillOpacity": 50, "lineWidth": 0},
          "unit": "short",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "desc"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum by (instance) (rate(node_network_receive_errs_total{instance=~\"$node\", device!~\"lo|veth.*|docker.*|cali.*|flannel.*\"}[5m])) + sum by (instance) (rate(node_network_receive_drop_total{instance=~\"$node\", device!~\"lo|veth.*|docker.*|cali.*|flannel.*\"}[5m]))",
          "legendFormat": "{{instance}} rx",
          "refId": "A"
        },
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum by (instance) (rate(node_network_transmit_errs_total{instance=~\"$node\", device!~\"lo|veth.*|docker.*|cali.*|flannel.*\"}[5m])) + sum by (instance) (rate(node_network_transmit_drop_total{instance=~\"$node\", device!~\"lo|veth.*|docker.*|cali.*|flannel.*\"}[5m]))",
          "legendFormat": "{{instance}} tx",
          "refId": "B"
        }
      ]
    },
    {"id": 240, "type": "row", "title": "System", "collapsed": false, "panels": [], "gridPos": {"h": 1, "w": 24, "x": 0, "y": 56}},
    {
      "id": 5,
      "type": "timeseries",
      "title": "Node Load Average",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 57},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "short",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "node_load1{instance=~\"$node\"}",
          "legendFormat": "{{instance}} load1",
          "refId": "A"
        },
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "node_load5{instance=~\"$node\"}",
          "legendFormat": "{{instance}} load5",
          "refId": "B"
        }
      ]
    },
    {
      "id": 112,
      "type": "timeseries",
      "title": "Context Switches & Interrupts",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 64},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "short",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "rate(node_context_switches_total{instance=~\"$node\"}[5m])",
          "legendFormat": "{{instance}} context switches",
          "refId": "A"
        },
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "rate(node_intr_total{instance=~\"$node\"}[5m])",
          "legendFormat": "{{instance}} interrupts",
          "refId": "B"
        }
      ]
    },
    {"id": 250, "type": "row", "title": "GPU", "collapsed": false, "panels": [], "gridPos": {"h": 1, "w": 24, "x": 0, "y": 71}},
    {
      "id": 113,
      "type": "timeseries",
      "title": "GPU Utilization",
      "description": "DCGM_FI_DEV_GPU_UTIL per GPU device. Empty if no GPU nodes are in this cluster, or if the GPU Operator's dcgm-exporter isn't being scraped yet.",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 72},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "percent",
          "min": 0,
          "max": 100
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "desc"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "DCGM_FI_DEV_GPU_UTIL{Hostname=~\"$gpu_host\"}",
          "legendFormat": "{{Hostname}} GPU {{gpu}}",
          "refId": "A"
        }
      ]
    },
    {
      "id": 114,
      "type": "timeseries",
      "title": "GPU Memory Used",
      "description": "DCGM_FI_DEV_FB_USED per GPU device (framebuffer memory, DCGM's native unit is MiB).",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 79},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "mbytes",
          "min": 0
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "desc"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "DCGM_FI_DEV_FB_USED{Hostname=~\"$gpu_host\"}",
          "legendFormat": "{{Hostname}} GPU {{gpu}}",
          "refId": "A"
        }
      ]
    }
  ]
}`
