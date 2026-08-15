# 由 `relaygate render --observability` 从本模板渲染；勿手工改 prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    gateway: ${GATEWAY_NAME}
    role: gateway

rule_files:
  - /etc/prometheus/rules/*.yml

scrape_configs:
  - job_name: envoy
    metrics_path: /stats/prometheus
    static_configs:
      - targets:
          - 127.0.0.1:${ENVOY_ADMIN_PORT}
        labels:
          role: gateway
          gateway: ${GATEWAY_NAME}
          host: ${GATEWAY_NAME}

  - job_name: node
    static_configs:
      - targets:
          - 127.0.0.1:9100
        labels:
          role: host
          gateway: ${GATEWAY_NAME}
          host: ${GATEWAY_NAME}

  - job_name: prometheus
    static_configs:
      - targets:
          - 127.0.0.1:9090
        labels:
          role: observability
          gateway: ${GATEWAY_NAME}
          host: ${GATEWAY_NAME}

# 可选：remote_write 到主控（节点 .env 设置 PROMETHEUS_REMOTE_WRITE_URL）
# 主控 Grafana 只读本机 Prometheus；节点须上报，例如：
#   PROMETHEUS_REMOTE_WRITE_URL=http://203.0.113.10:9000/api/agent/metrics/write
# remote_write:
#   - url: ${PROMETHEUS_REMOTE_WRITE_URL}
#     queue_config:
#       max_samples_per_send: 1000
#       capacity: 10000
