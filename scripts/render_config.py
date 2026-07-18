#!/usr/bin/env python3
"""从 config/resources.yaml 渲染 Envoy 配置与 nftables 端口清单。"""

from __future__ import annotations

import argparse
import ipaddress
import sys
from pathlib import Path

try:
    import yaml
except ImportError:
    print("缺少 PyYAML，请执行: pip install pyyaml", file=sys.stderr)
    sys.exit(1)

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_RESOURCES = ROOT / "config" / "resources.yaml"
DEFAULT_ENVOY_OUT = ROOT / "envoy" / "generated" / "envoy.yaml"
DEFAULT_NFT_PORTS_OUT = ROOT / "firewall" / "generated" / "game-ports.nft"


def die(msg: str) -> None:
    print(f"ERROR: {msg}", file=sys.stderr)
    sys.exit(1)


def load_resources(path: Path) -> dict:
    if not path.exists():
        die(f"资产文件不存在: {path}")
    with path.open("r", encoding="utf-8") as f:
        data = yaml.safe_load(f)
    if not isinstance(data, dict):
        die("resources.yaml 根节点必须是 mapping")
    return data


def validate(data: dict) -> tuple[dict[str, dict], list[dict]]:
    servers = {s["name"]: s for s in data.get("servers", [])}
    if not servers:
        die("servers 不能为空")

    for name, s in servers.items():
        try:
            ipaddress.ip_address(s["address"])
        except Exception as exc:  # noqa: BLE001
            die(f"{name} address 无效: {s.get('address')} ({exc})")
        for key in ("tcp_port", "udp_port"):
            port = int(s[key])
            if not 1 <= port <= 65535:
                die(f"{name}.{key} 端口越界: {port}")

    rules = [r for r in data.get("rules", []) if r.get("enabled", True)]
    seen: dict[tuple[str, int], str] = {}
    for rule in rules:
        name = rule["name"]
        proto = rule["protocol"].upper()
        if proto not in ("TCP", "UDP"):
            die(f"{name}: protocol 必须是 TCP 或 UDP")
        port = int(rule["listen_port"])
        if not 1 <= port <= 65535:
            die(f"{name}: listen_port 越界: {port}")
        key = (proto, port)
        if key in seen:
            die(f"端口冲突: {proto}/{port} 同时被 {seen[key]} 与 {name} 使用")
        seen[key] = name
        server = rule["server"]
        if server not in servers:
            die(f"{name}: 未知 server {server}")
        if not servers[server].get("enabled", True):
            die(f"{name}: 目标 {server} 已禁用")

    return servers, rules


def cluster_name(server: str, protocol: str) -> str:
    return f"cluster-{server}-{protocol.lower()}-game"


def listener_name(rule_name: str) -> str:
    return f"listener-{rule_name}"


def render_tcp_cluster(server: dict, defaults: dict) -> dict:
    name = cluster_name(server["name"], "TCP")
    hc = defaults["health_check"]
    return {
        "name": name,
        "type": "STATIC",
        "connect_timeout": "2s",
        "lb_policy": "ROUND_ROBIN",
        "circuit_breakers": {
            "thresholds": [
                {
                    "priority": "DEFAULT",
                    "max_connections": defaults["max_connections"],
                    "max_pending_requests": defaults["max_pending_requests"],
                    "max_requests": defaults["max_connections"],
                }
            ]
        },
        "health_checks": [
            {
                "timeout": hc["timeout"],
                "interval": hc["interval"],
                "unhealthy_threshold": hc["unhealthy_threshold"],
                "healthy_threshold": hc["healthy_threshold"],
                "tcp_health_check": {},
            }
        ],
        "load_assignment": {
            "cluster_name": name,
            "endpoints": [
                {
                    "lb_endpoints": [
                        {
                            "endpoint": {
                                "address": {
                                    "socket_address": {
                                        "address": server["address"],
                                        "port_value": int(server["tcp_port"]),
                                    }
                                },
                                "health_check_config": {
                                    "port_value": int(
                                        server.get(
                                            "health_check_port", server["tcp_port"]
                                        )
                                    )
                                },
                            }
                        }
                    ]
                }
            ],
        },
    }


def render_udp_cluster(server: dict, defaults: dict) -> dict:
    name = cluster_name(server["name"], "UDP")
    return {
        "name": name,
        "type": "STATIC",
        "connect_timeout": "2s",
        "lb_policy": "ROUND_ROBIN",
        "circuit_breakers": {
            "thresholds": [
                {
                    "priority": "DEFAULT",
                    # UDP proxy 用 max_connections 限制会话数
                    "max_connections": defaults["max_connections"],
                }
            ]
        },
        "load_assignment": {
            "cluster_name": name,
            "endpoints": [
                {
                    "lb_endpoints": [
                        {
                            "endpoint": {
                                "address": {
                                    "socket_address": {
                                        "address": server["address"],
                                        "port_value": int(server["udp_port"]),
                                    }
                                }
                            }
                        }
                    ]
                }
            ],
        },
    }


def render_tcp_listener(rule: dict, listen_address: str, defaults: dict) -> dict:
    cname = cluster_name(rule["server"], "TCP")
    prefix = rule["name"].replace("-", "_")
    return {
        "name": listener_name(rule["name"]),
        "address": {
            "socket_address": {
                "protocol": "TCP",
                "address": listen_address,
                "port_value": int(rule["listen_port"]),
            }
        },
        "filter_chains": [
            {
                "filters": [
                    {
                        "name": "envoy.filters.network.local_ratelimit",
                        "typed_config": {
                            "@type": "type.googleapis.com/envoy.extensions.filters.network.local_ratelimit.v3.LocalRateLimit",
                            "stat_prefix": f"rl_{prefix}",
                            "token_bucket": {
                                "max_tokens": int(defaults["tcp_local_rate_limit_burst"]),
                                "tokens_per_fill": int(
                                    defaults["tcp_local_rate_limit_per_sec"]
                                ),
                                "fill_interval": "1s",
                            },
                        },
                    },
                    {
                        "name": "envoy.filters.network.tcp_proxy",
                        "typed_config": {
                            "@type": "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy",
                            "stat_prefix": f"tcp_{prefix}",
                            "cluster": cname,
                            "idle_timeout": defaults["tcp_idle_timeout"],
                            "access_log": [
                                {
                                    "name": "envoy.access_loggers.file",
                                    "typed_config": {
                                        "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
                                        "path": "/var/log/envoy/tcp-access.json",
                                        "log_format": {
                                            "json_format": {
                                                "ts": "%START_TIME%",
                                                "rule": rule["name"],
                                                "protocol": "TCP",
                                                "downstream": "%DOWNSTREAM_REMOTE_ADDRESS%",
                                                "upstream": "%UPSTREAM_HOST%",
                                                "bytes_rx": "%BYTES_RECEIVED%",
                                                "bytes_tx": "%BYTES_SENT%",
                                                "duration_ms": "%DURATION%",
                                                "flags": "%RESPONSE_FLAGS%",
                                            }
                                        },
                                    },
                                }
                            ],
                        },
                    },
                ]
            }
        ],
    }


def render_udp_listener(rule: dict, listen_address: str, defaults: dict) -> dict:
    cname = cluster_name(rule["server"], "UDP")
    prefix = rule["name"].replace("-", "_")
    return {
        "name": listener_name(rule["name"]),
        "address": {
            "socket_address": {
                "protocol": "UDP",
                "address": listen_address,
                "port_value": int(rule["listen_port"]),
            }
        },
        "udp_listener_config": {
            "downstream_socket_config": {
                "max_rx_datagram_size": 1500,
            }
        },
        "listener_filters": [
            {
                "name": "envoy.filters.udp_listener.udp_proxy",
                "typed_config": {
                    "@type": "type.googleapis.com/envoy.extensions.filters.udp.udp_proxy.v3.UdpProxyConfig",
                    "stat_prefix": f"udp_{prefix}",
                    "idle_timeout": defaults["udp_idle_timeout"],
                    "matcher": {
                        "on_no_match": {
                            "action": {
                                "name": "route",
                                "typed_config": {
                                    "@type": "type.googleapis.com/envoy.extensions.filters.udp.udp_proxy.v3.Route",
                                    "cluster": cname,
                                },
                            }
                        }
                    },
                    "upstream_socket_config": {
                        "max_rx_datagram_size": 1500,
                    },
                },
            }
        ],
    }


def build_envoy_config(data: dict, servers: dict[str, dict], rules: list[dict]) -> dict:
    defaults = data["defaults"]
    meta = data["meta"]
    listen_address = data["gateway"]["listen_address"]

    needed_clusters: dict[str, dict] = {}
    listeners: list[dict] = []

    for rule in rules:
        server = servers[rule["server"]]
        proto = rule["protocol"].upper()
        cname = cluster_name(server["name"], proto)
        if cname not in needed_clusters:
            if proto == "TCP":
                needed_clusters[cname] = render_tcp_cluster(server, defaults)
            else:
                needed_clusters[cname] = render_udp_cluster(server, defaults)
        if proto == "TCP":
            listeners.append(render_tcp_listener(rule, listen_address, defaults))
        else:
            listeners.append(render_udp_listener(rule, listen_address, defaults))

    return {
        "admin": {
            "address": {
                "socket_address": {
                    "protocol": "TCP",
                    "address": meta["admin_address"],
                    "port_value": int(meta["admin_port"]),
                }
            },
            "access_log": [
                {
                    "name": "envoy.access_loggers.file",
                    "typed_config": {
                        "@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
                        "path": "/var/log/envoy/admin-access.log",
                    },
                }
            ],
        },
        "static_resources": {
            "listeners": listeners,
            "clusters": list(needed_clusters.values()),
        },
    }


def render_nft_ports(rules: list[dict]) -> str:
    tcp_ports = sorted({int(r["listen_port"]) for r in rules if r["protocol"].upper() == "TCP"})
    udp_ports = sorted({int(r["listen_port"]) for r in rules if r["protocol"].upper() == "UDP"})
    tcp_csv = ", ".join(str(p) for p in tcp_ports) if tcp_ports else "10001"
    udp_csv = ", ".join(str(p) for p in udp_ports) if udp_ports else "10001"
    return f"""# 由 scripts/render_config.py 自动生成，勿手改
# TCP game ports
define GAME_TCP_PORTS = {{ {tcp_csv} }}
# UDP game ports
define GAME_UDP_PORTS = {{ {udp_csv} }}
"""


def write_yaml(path: Path, data: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as f:
        f.write("# 由 scripts/render_config.py 自动生成，勿手改\n")
        f.write("# 源文件: config/resources.yaml\n")
        yaml.safe_dump(
            data,
            f,
            sort_keys=False,
            allow_unicode=True,
            default_flow_style=False,
        )


def main() -> None:
    parser = argparse.ArgumentParser(description="渲染 Envoy / nftables 配置")
    parser.add_argument("--resources", type=Path, default=DEFAULT_RESOURCES)
    parser.add_argument("--envoy-out", type=Path, default=DEFAULT_ENVOY_OUT)
    parser.add_argument("--nft-out", type=Path, default=DEFAULT_NFT_PORTS_OUT)
    parser.add_argument(
        "--check-only",
        action="store_true",
        help="仅校验，不写文件",
    )
    args = parser.parse_args()

    data = load_resources(args.resources)
    servers, rules = validate(data)

    if not rules:
        die("没有启用的 rules；至少启用 canary 规则后再渲染")

    config = build_envoy_config(data, servers, rules)
    nft = render_nft_ports(rules)

    print(f"校验通过: {len(servers)} 台服务器, {len(rules)} 条启用规则")
    for rule in rules:
        print(
            f"  - {rule['name']}: {rule['protocol']}/{rule['listen_port']} -> "
            f"{rule['server']} ({rule.get('kind', 'unknown')})"
        )

    if args.check_only:
        return

    write_yaml(args.envoy_out, config)
    args.nft_out.parent.mkdir(parents=True, exist_ok=True)
    args.nft_out.write_text(nft, encoding="utf-8")
    print(f"已写入 {args.envoy_out}")
    print(f"已写入 {args.nft_out}")


if __name__ == "__main__":
    main()
