#!/usr/bin/env python3
"""启用/禁用某台游戏服的 production 规则（保留 canary），尽量保留 YAML 注释。"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

try:
    import yaml
except ImportError:
    print("缺少 PyYAML，请: pip install pyyaml", file=sys.stderr)
    sys.exit(1)

ROOT = Path(__file__).resolve().parents[1]
RESOURCES = ROOT / "config" / "resources.yaml"


def main() -> None:
    parser = argparse.ArgumentParser(description="启用/禁用 production 规则")
    parser.add_argument("server", nargs="?", default="", help="例如 server-01；配合 --all-production 可省略")
    parser.add_argument(
        "--disable",
        action="store_true",
        help="禁用该服 production 规则（默认启用）",
    )
    parser.add_argument(
        "--all-production",
        action="store_true",
        help="对全部 production 规则生效",
    )
    args = parser.parse_args()

    if not args.all_production and not args.server:
        parser.error("请指定 server，或使用 --all-production")

    text = RESOURCES.read_text(encoding="utf-8")
    data = yaml.safe_load(text)

    target_names: list[str] = []
    for rule in data.get("rules", []):
        if rule.get("kind") != "production":
            continue
        if not args.all_production and rule.get("server") != args.server:
            continue
        target_names.append(rule["name"])

    if not target_names:
        print("没有匹配的 production 规则（检查 server 名）")
        sys.exit(1)

    enabled_value = "false" if args.disable else "true"
    changed = 0

    for name in target_names:
        # 在对应 rule 块内替换 enabled 字段
        pattern = re.compile(
            rf"(^[ \t]*- name:\s*{re.escape(name)}\s*\n(?:^[ \t]+.*\n)*?^[ \t]+enabled:\s*)(true|false)",
            re.MULTILINE,
        )

        def repl(m: re.Match[str], _name: str = name) -> str:
            nonlocal changed
            old = m.group(2)
            if old != enabled_value:
                changed += 1
                state = "enabled" if enabled_value == "true" else "disabled"
                print(f"{state}: {_name}")
            return m.group(1) + enabled_value

        new_text, n = pattern.subn(repl, text, count=1)
        if n == 0:
            print(f"WARN: 未能在文本中定位规则块: {name}", file=sys.stderr)
        text = new_text

    if changed == 0:
        print("没有规则被修改（可能已是目标状态）")
        return

    RESOURCES.write_text(text, encoding="utf-8")
    print(f"已更新 {RESOURCES}（{changed} 条）")
    print("请执行: python scripts/render_config.py && bash scripts/deploy.sh")


if __name__ == "__main__":
    main()
