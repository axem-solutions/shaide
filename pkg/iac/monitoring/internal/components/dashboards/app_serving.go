package dashboards

const appServingDashboard = `{
  "uid": "app-serving-logs",
  "title": "app-serving · Model Log Explorer",
  "tags": ["app-serving", "logs"],
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
      },
      {
        "name": "model_category",
        "label": "Category",
        "type": "query",
        "datasource": {"type": "loki", "uid": "${datasource}"},
        "query": {"label": "model_category", "stream": "{part_of=\"app-serving\"}", "type": 1},
        "refresh": 2,
        "sort": 1,
        "multi": false,
        "includeAll": true,
        "allValue": ".*",
        "hide": 0
      },
      {
        "name": "model_slug",
        "label": "Model",
        "type": "query",
        "datasource": {"type": "loki", "uid": "${datasource}"},
        "query": {"label": "model_slug", "stream": "{part_of=\"app-serving\", model_category=~\"$model_category\"}", "type": 1},
        "refresh": 2,
        "sort": 1,
        "multi": true,
        "includeAll": true,
        "allValue": ".*",
        "hide": 0
      },
      {
        "name": "nodegroup",
        "label": "Nodegroup",
        "type": "query",
        "datasource": {"type": "loki", "uid": "${datasource}"},
        "query": {"label": "nodegroup", "stream": "{part_of=\"app-serving\", model_category=~\"$model_category\", model_slug=~\"$model_slug\"}", "type": 1},
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
    {
      "id": 1,
      "type": "timeseries",
      "title": "Log Rate by Model",
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
          "expr": "sum(rate({part_of=\"app-serving\", model_slug=~\"$model_slug\", model_category=~\"$model_category\", nodegroup=~\"$nodegroup\"}[5m])) by (model_slug)",
          "legendFormat": "{{model_slug}}",
          "refId": "A",
          "queryType": "range"
        }
      ]
    },
    {
      "id": 3,
      "type": "timeseries",
      "title": "Log Rate by Nodegroup",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 7},
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
          "expr": "sum(rate({part_of=\"app-serving\", model_slug=~\"$model_slug\", model_category=~\"$model_category\", nodegroup=~\"$nodegroup\"}[5m])) by (nodegroup)",
          "legendFormat": "{{nodegroup}}",
          "refId": "A",
          "queryType": "range"
        }
      ]
    },
    {
      "id": 2,
      "type": "logs",
      "title": "Log Stream",
      "gridPos": {"h": 20, "w": 24, "x": 0, "y": 14},
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
          "expr": "{part_of=\"app-serving\", model_slug=~\"$model_slug\", model_category=~\"$model_category\", nodegroup=~\"$nodegroup\"}",
          "refId": "A",
          "queryType": "range"
        }
      ]
    }
  ]
}`
