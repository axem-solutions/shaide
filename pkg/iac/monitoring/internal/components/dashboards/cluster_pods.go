package dashboards

const clusterPodsDashboard = `{
  "uid": "cluster-pods",
  "title": "Cluster · Pod Resource Usage",
  "tags": ["cluster", "metrics", "pods"],
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
        "name": "namespace",
        "label": "Namespace",
        "type": "query",
        "datasource": {"type": "prometheus", "uid": "${datasource}"},
        "query": "label_values(kube_pod_info, namespace)",
        "refresh": 2,
        "sort": 1,
        "multi": true,
        "includeAll": true,
        "allValue": ".*",
        "hide": 0
      },
      {
        "name": "pod",
        "label": "Pod",
        "type": "query",
        "datasource": {"type": "prometheus", "uid": "${datasource}"},
        "query": "label_values(kube_pod_info{namespace=~\"$namespace\"}, pod)",
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
    {"id": 20, "type": "row", "title": "Overview", "collapsed": false, "panels": [], "gridPos": {"h": 1, "w": 24, "x": 0, "y": 0}},
    {
      "id": 21,
      "type": "gauge",
      "title": "CPU Usage vs. Requests",
      "description": "Actual CPU usage divided by declared requests, for currently-running pods only. Sustained values well below 1 mean requests are set too high (wasted scheduling headroom); values above 1 mean the pod is bursting past what was reserved.",
      "gridPos": {"h": 6, "w": 6, "x": 0, "y": 1},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "unit": "percentunit",
          "min": 0,
          "max": 1.5,
          "thresholds": {"mode": "absolute", "steps": [{"color": "green", "value": null}, {"color": "yellow", "value": 0.8}, {"color": "red", "value": 1}]}
        },
        "overrides": []
      },
      "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "orientation": "auto", "showThresholdLabels": false, "showThresholdMarkers": true},
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum(rate(container_cpu_usage_seconds_total{namespace=~\"$namespace\", pod=~\"$pod\", container!=\"\", container!=\"POD\"}[5m])) / sum(kube_pod_container_resource_requests{namespace=~\"$namespace\", pod=~\"$pod\", resource=\"cpu\"} and on(namespace, pod) max by (namespace, pod) (kube_pod_status_phase{phase=\"Running\", namespace=~\"$namespace\", pod=~\"$pod\"} == 1))",
          "refId": "A"
        }
      ]
    },
    {
      "id": 22,
      "type": "gauge",
      "title": "CPU Usage vs. Limits",
      "description": "Actual CPU usage divided by declared limits, for currently-running pods only. Values near 1 mean the pod is close to being CPU-throttled.",
      "gridPos": {"h": 6, "w": 6, "x": 6, "y": 1},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "unit": "percentunit",
          "min": 0,
          "max": 1.5,
          "thresholds": {"mode": "absolute", "steps": [{"color": "green", "value": null}, {"color": "yellow", "value": 0.8}, {"color": "red", "value": 1}]}
        },
        "overrides": []
      },
      "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "orientation": "auto", "showThresholdLabels": false, "showThresholdMarkers": true},
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum(rate(container_cpu_usage_seconds_total{namespace=~\"$namespace\", pod=~\"$pod\", container!=\"\", container!=\"POD\"}[5m])) / sum(kube_pod_container_resource_limits{namespace=~\"$namespace\", pod=~\"$pod\", resource=\"cpu\"} and on(namespace, pod) max by (namespace, pod) (kube_pod_status_phase{phase=\"Running\", namespace=~\"$namespace\", pod=~\"$pod\"} == 1))",
          "refId": "A"
        }
      ]
    },
    {
      "id": 23,
      "type": "gauge",
      "title": "RAM Usage vs. Requests",
      "description": "Actual memory usage divided by declared requests, for currently-running pods only.",
      "gridPos": {"h": 6, "w": 6, "x": 12, "y": 1},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "unit": "percentunit",
          "min": 0,
          "max": 1.5,
          "thresholds": {"mode": "absolute", "steps": [{"color": "green", "value": null}, {"color": "yellow", "value": 0.8}, {"color": "red", "value": 1}]}
        },
        "overrides": []
      },
      "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "orientation": "auto", "showThresholdLabels": false, "showThresholdMarkers": true},
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum(container_memory_working_set_bytes{namespace=~\"$namespace\", pod=~\"$pod\", container!=\"\", container!=\"POD\"}) / sum(kube_pod_container_resource_requests{namespace=~\"$namespace\", pod=~\"$pod\", resource=\"memory\"} and on(namespace, pod) max by (namespace, pod) (kube_pod_status_phase{phase=\"Running\", namespace=~\"$namespace\", pod=~\"$pod\"} == 1))",
          "refId": "A"
        }
      ]
    },
    {
      "id": 24,
      "type": "gauge",
      "title": "RAM Usage vs. Limits",
      "description": "Actual memory usage divided by declared limits, for currently-running pods only. Values near 1 mean the pod is at risk of OOMKill.",
      "gridPos": {"h": 6, "w": 6, "x": 18, "y": 1},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "unit": "percentunit",
          "min": 0,
          "max": 1.5,
          "thresholds": {"mode": "absolute", "steps": [{"color": "green", "value": null}, {"color": "yellow", "value": 0.8}, {"color": "red", "value": 1}]}
        },
        "overrides": []
      },
      "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "orientation": "auto", "showThresholdLabels": false, "showThresholdMarkers": true},
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum(container_memory_working_set_bytes{namespace=~\"$namespace\", pod=~\"$pod\", container!=\"\", container!=\"POD\"}) / sum(kube_pod_container_resource_limits{namespace=~\"$namespace\", pod=~\"$pod\", resource=\"memory\"} and on(namespace, pod) max by (namespace, pod) (kube_pod_status_phase{phase=\"Running\", namespace=~\"$namespace\", pod=~\"$pod\"} == 1))",
          "refId": "A"
        }
      ]
    },
    {"id": 25, "type": "row", "title": "Resource Usage", "collapsed": false, "panels": [], "gridPos": {"h": 1, "w": 24, "x": 0, "y": 7}},
    {
      "id": 1,
      "type": "timeseries",
      "title": "CPU Usage by Pod (cores)",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 8},
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
          "expr": "sum by (namespace, pod) (rate(container_cpu_usage_seconds_total{namespace=~\"$namespace\", pod=~\"$pod\", container!=\"\", container!=\"POD\"}[5m]))",
          "legendFormat": "{{namespace}} / {{pod}}",
          "refId": "A"
        }
      ]
    },
    {
      "id": 2,
      "type": "timeseries",
      "title": "Memory Usage by Pod",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 15},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "line", "fillOpacity": 10, "lineWidth": 1},
          "unit": "bytes",
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
          "expr": "sum by (namespace, pod) (container_memory_working_set_bytes{namespace=~\"$namespace\", pod=~\"$pod\", container!=\"\", container!=\"POD\"})",
          "legendFormat": "{{namespace}} / {{pod}}",
          "refId": "A"
        }
      ]
    },
    {
      "id": 26,
      "type": "timeseries",
      "title": "CPU Throttled Time by Container",
      "description": "Fraction of time each container spent throttled by its CPU limit — the actionable signal for whether a limit is set too tight, distinct from raw usage.",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 22},
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
        "tooltip": {"mode": "multi", "sort": "desc"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum by (namespace, pod, container) (rate(container_cpu_cfs_throttled_seconds_total{namespace=~\"$namespace\", pod=~\"$pod\", container!=\"\", container!=\"POD\"}[5m]))",
          "legendFormat": "{{namespace}} / {{pod}} / {{container}}",
          "refId": "A"
        }
      ]
    },
    {"id": 27, "type": "row", "title": "Health", "collapsed": false, "panels": [], "gridPos": {"h": 1, "w": 24, "x": 0, "y": 29}},
    {
      "id": 3,
      "type": "timeseries",
      "title": "Pod Restarts",
      "gridPos": {"h": 7, "w": 12, "x": 0, "y": 30},
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
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum by (namespace, pod) (increase(kube_pod_container_status_restarts_total{namespace=~\"$namespace\", pod=~\"$pod\"}[5m]))",
          "legendFormat": "{{namespace}} / {{pod}}",
          "refId": "A"
        }
      ]
    },
    {
      "id": 4,
      "type": "stat",
      "title": "Pods Not Ready",
      "gridPos": {"h": 7, "w": 12, "x": 12, "y": 30},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "unit": "short",
          "thresholds": {
            "mode": "absolute",
            "steps": [
              {"color": "green", "value": null},
              {"color": "red", "value": 1}
            ]
          }
        },
        "overrides": []
      },
      "options": {
        "reduceOptions": {"calcs": ["lastNotNull"]},
        "colorMode": "value",
        "graphMode": "none"
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum(kube_pod_status_ready{condition=\"false\", namespace=~\"$namespace\"}) or vector(0)",
          "refId": "A"
        }
      ]
    },
    {
      "id": 28,
      "type": "table",
      "title": "Pods with Container Issues",
      "description": "Containers currently waiting on ErrImagePull, ImagePullBackOff, or CrashLoopBackOff.",
      "gridPos": {"h": 7, "w": 12, "x": 0, "y": 37},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {"defaults": {}, "overrides": []},
      "options": {"showHeader": true},
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "kube_pod_container_status_waiting_reason{namespace=~\"$namespace\", pod=~\"$pod\", reason=~\"ErrImagePull|ImagePullBackOff|CrashLoopBackOff\"} == 1",
          "format": "table",
          "instant": true,
          "refId": "A"
        }
      ]
    },
    {
      "id": 29,
      "type": "table",
      "title": "Unscheduled Pods",
      "description": "Pods the scheduler has not yet placed on a node — usually insufficient CPU/memory/GPU capacity or an unsatisfiable nodeSelector.",
      "gridPos": {"h": 7, "w": 12, "x": 12, "y": 37},
      "datasource": {"type": "prometheus", "uid": "${datasource}"},
      "fieldConfig": {"defaults": {}, "overrides": []},
      "options": {"showHeader": true},
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "kube_pod_status_scheduled{namespace=~\"$namespace\", pod=~\"$pod\", condition=\"false\"} == 1",
          "format": "table",
          "instant": true,
          "refId": "A"
        }
      ]
    },
    {
      "id": 30,
      "type": "timeseries",
      "title": "OOM Events by Container",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 44},
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
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "sum by (namespace, pod, container) (increase(container_oom_events_total{namespace=~\"$namespace\", pod=~\"$pod\", container!=\"\"}[5m]))",
          "legendFormat": "{{namespace}} / {{pod}} / {{container}}",
          "refId": "A"
        }
      ]
    },
    {"id": 31, "type": "row", "title": "Network", "collapsed": false, "panels": [], "gridPos": {"h": 1, "w": 24, "x": 0, "y": 51}},
    {
      "id": 32,
      "type": "timeseries",
      "title": "Network Bandwidth by Pod",
      "description": "Receive above the axis, transmit mirrored below — a common idiom for visualizing bidirectional traffic in one panel.",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 52},
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
          "expr": "sum by (namespace, pod) (rate(container_network_receive_bytes_total{namespace=~\"$namespace\", pod=~\"$pod\"}[5m]))",
          "legendFormat": "{{namespace}} / {{pod}} rx",
          "refId": "A"
        },
        {
          "datasource": {"type": "prometheus", "uid": "${datasource}"},
          "expr": "-sum by (namespace, pod) (rate(container_network_transmit_bytes_total{namespace=~\"$namespace\", pod=~\"$pod\"}[5m]))",
          "legendFormat": "{{namespace}} / {{pod}} tx",
          "refId": "B"
        }
      ]
    }
  ]
}`
