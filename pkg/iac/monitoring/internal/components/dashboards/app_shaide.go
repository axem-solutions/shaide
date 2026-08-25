package dashboards

const appShaideDashboard = `{
  "uid": "app-shaide-logs",
  "title": "app-shaide · Log Explorer",
  "tags": ["app-shaide", "logs"],
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
        "name": "component",
        "label": "Component",
        "type": "query",
        "datasource": {"type": "loki", "uid": "${datasource}"},
        "query": {"label": "component", "stream": "{part_of=\"app-shaide\"}", "type": 1},
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
      "title": "Log Rate by Component",
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
          "expr": "sum(rate({part_of=\"app-shaide\", component=~\"$component\"}[5m])) by (component)",
          "legendFormat": "{{component}}",
          "refId": "A",
          "queryType": "range"
        }
      ]
    },
    {
      "id": 2,
      "type": "logs",
      "title": "Log Stream",
      "gridPos": {"h": 20, "w": 24, "x": 0, "y": 7},
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
          "expr": "{part_of=\"app-shaide\", component=~\"$component\"}",
          "refId": "A",
          "queryType": "range"
        }
      ]
    }
  ]
}`
