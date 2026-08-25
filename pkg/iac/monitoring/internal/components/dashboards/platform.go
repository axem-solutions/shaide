package dashboards

const platformDashboard = `{
  "uid": "platform-overview",
  "title": "AI Platform · Overview",
  "tags": ["platform", "logs"],
  "schemaVersion": 38,
  "time": {"from": "now-1h", "to": "now"},
  "refresh": "5s",
  "templating": {
    "list": [
      {
        "name": "datasource",
        "type": "datasource",
        "pluginId": "loki",
        "label": "Data source",
        "refresh": 1,
        "hide": 0
      }
    ]
  },
  "panels": [
    {
      "id": 1,
      "type": "timeseries",
      "title": "Platform Log Rate",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 0},
      "datasource": {"type": "loki", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "bars", "fillOpacity": 50, "lineWidth": 0},
          "unit": "short"
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "loki", "uid": "${datasource}"},
          "expr": "sum(rate({platform=\"ai-platform\"}[5m])) by (part_of)",
          "legendFormat": "{{part_of}}",
          "refId": "A",
          "queryType": "range"
        }
      ]
    },
    {
      "id": 2,
      "type": "timeseries",
      "title": "app-shaide · Rate by Component",
      "gridPos": {"h": 7, "w": 8, "x": 0, "y": 7},
      "datasource": {"type": "loki", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "bars", "fillOpacity": 50, "lineWidth": 0},
          "unit": "short"
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "loki", "uid": "${datasource}"},
          "expr": "sum(rate({part_of=\"app-shaide\"}[5m])) by (component)",
          "legendFormat": "{{component}}",
          "refId": "A",
          "queryType": "range"
        }
      ]
    },
    {
      "id": 3,
      "type": "timeseries",
      "title": "app-serving · Rate by Model",
      "gridPos": {"h": 7, "w": 8, "x": 8, "y": 7},
      "datasource": {"type": "loki", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "bars", "fillOpacity": 50, "lineWidth": 0},
          "unit": "short"
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "loki", "uid": "${datasource}"},
          "expr": "sum(rate({part_of=\"app-serving\"}[5m])) by (model_slug)",
          "legendFormat": "{{model_slug}}",
          "refId": "A",
          "queryType": "range"
        }
      ]
    },
    {
      "id": 6,
      "type": "timeseries",
      "title": "app-serving · Rate by Category",
      "gridPos": {"h": 7, "w": 8, "x": 16, "y": 7},
      "datasource": {"type": "loki", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "custom": {"drawStyle": "bars", "fillOpacity": 50, "lineWidth": 0},
          "unit": "short"
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "loki", "uid": "${datasource}"},
          "expr": "sum(rate({part_of=\"app-serving\"}[5m])) by (model_category)",
          "legendFormat": "{{model_category}}",
          "refId": "A",
          "queryType": "range"
        }
      ]
    },
    {
      "id": 4,
      "type": "timeseries",
      "title": "Error Rate by Component",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 14},
      "datasource": {"type": "loki", "uid": "${datasource}"},
      "fieldConfig": {
        "defaults": {
          "color": {"fixedColor": "red", "mode": "fixed"},
          "custom": {"drawStyle": "bars", "fillOpacity": 50, "lineWidth": 0},
          "unit": "short"
        },
        "overrides": []
      },
      "options": {
        "legend": {"displayMode": "list", "placement": "bottom", "calcs": []},
        "tooltip": {"mode": "multi", "sort": "none"}
      },
      "targets": [
        {
          "datasource": {"type": "loki", "uid": "${datasource}"},
          "expr": "sum(rate({platform=\"ai-platform\"} |~ \"(?i)(error|fatal|panic)\"[5m])) by (part_of, component)",
          "legendFormat": "{{part_of}} / {{component}}",
          "refId": "A",
          "queryType": "range"
        }
      ]
    },
    {
      "id": 5,
      "type": "logs",
      "title": "Error Log Stream",
      "gridPos": {"h": 20, "w": 24, "x": 0, "y": 21},
      "datasource": {"type": "loki", "uid": "${datasource}"},
      "options": {
        "dedupStrategy": "none",
        "enableLogDetails": true,
        "showTime": true,
        "sortOrder": "Descending",
        "wrapLogMessage": false
      },
      "targets": [
        {
          "datasource": {"type": "loki", "uid": "${datasource}"},
          "expr": "{platform=\"ai-platform\"} |~ \"(?i)(error|fatal|panic)\"",
          "refId": "A",
          "queryType": "range"
        }
      ]
    }
  ]
}`
