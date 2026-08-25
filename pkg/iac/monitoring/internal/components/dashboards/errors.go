package dashboards

const errorsDashboard = `{
  "uid": "error-logs",
  "title": "AI Platform · Error Explorer",
  "tags": ["errors", "logs"],
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
        "name": "namespace",
        "label": "Namespace",
        "type": "query",
        "datasource": {"type": "loki", "uid": "${datasource}"},
        "query": {"label": "namespace", "stream": "{platform=\"ai-platform\"}", "type": 1},
        "refresh": 2,
        "sort": 1,
        "multi": true,
        "includeAll": true,
        "allValue": ".*",
        "hide": 0
      },
      {
        "name": "component",
        "label": "Component",
        "type": "query",
        "datasource": {"type": "loki", "uid": "${datasource}"},
        "query": {"label": "component", "stream": "{platform=\"ai-platform\"}", "type": 1},
        "refresh": 2,
        "sort": 1,
        "multi": true,
        "includeAll": true,
        "allValue": ".*",
        "hide": 0
      },
      {
        "name": "model_slug",
        "label": "Model",
        "type": "query",
        "datasource": {"type": "loki", "uid": "${datasource}"},
        "query": {"label": "model_slug", "stream": "{platform=\"ai-platform\"}", "type": 1},
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
      "title": "Error Rate by Component",
      "gridPos": {"h": 7, "w": 24, "x": 0, "y": 0},
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
          "expr": "sum(rate({platform=\"ai-platform\", namespace=~\"$namespace\", component=~\"$component\", model_slug=~\"$model_slug\"} |~ \"(?i)(error|fatal|panic)\"[5m])) by (namespace, component)",
          "legendFormat": "{{namespace}} / {{component}}",
          "refId": "A",
          "queryType": "range"
        }
      ]
    },
    {
      "id": 2,
      "type": "logs",
      "title": "Error Log Stream",
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
          "expr": "{platform=\"ai-platform\", namespace=~\"$namespace\", component=~\"$component\", model_slug=~\"$model_slug\"} |~ \"(?i)(error|fatal|panic)\"",
          "refId": "A",
          "queryType": "range"
        }
      ]
    }
  ]
}`
