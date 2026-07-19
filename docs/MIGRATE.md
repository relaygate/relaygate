# RelayGate 分批迁移手册（server-01 → server-10）

## 前置条件

1. 已在 `config/resources.yaml` 填入真实后端 IP
2. 已更换暴露过的 root 密码，并配置 SSH 密钥
3. `bash scripts/build.sh` 已生成 `bin/gateway-render`
4. `bash scripts/collect_baseline.sh` 已执行
5. canary 规则（11001）已通过游戏客户端验证

## 阶段 A：旁路 canary（默认已启用）

```bash
bash scripts/build.sh
cp .env.example .env && chmod 600 .env   # 改 Grafana / Panel 密码
bash scripts/deploy.sh
bash scripts/canary_test.sh 127.0.0.1
bash scripts/canary_test.sh 107.149.191.37
```

验证项：

- TCP 建连成功
- UDP 双向回包（游戏客户端）
- 长连接不掉
- 人为 `iptables -I OUTPUT -d <server-01-ip> -j DROP` 后，健康检查变红、告警触发
- 恢复规则后集群恢复 healthy
- Panel Overview / Grafana 有连接/会话/吞吐数据

## 阶段 B：逐台启用 production

对每台服（从 server-01 开始）：

```bash
./bin/gateway-render enable server-01
./bin/gateway-render
bash scripts/deploy.sh

# 2. 客户端改连 gateway-01:10001（TCP/UDP）
# 3. 观察 15–30 分钟指标与玩家反馈
# 4. 通过后再 enable server-02 ...
```

或通过 Panel：`Rules` 开关 → `Apply`。

启用全部：

```bash
./bin/gateway-render enable --all-production
./bin/gateway-render
bash scripts/deploy.sh
```

## 阶段 C：防火墙

仅在 canary/生产验证通过后：

```bash
sudo bash scripts/apply_firewall.sh
```

## 回滚

```bash
./bin/gateway-render disable server-03
bash scripts/deploy.sh

# 或整包回滚最近备份
bash scripts/rollback.sh
```

## 注意

- 固定目标模式下，熔断只会停止向故障服转发，不会让其他区服接管
- 后端看到的源 IP 是 `gateway-01`，需在游戏服防火墙放行该 IP
- 大流量 DDoS 仍需云厂商高防
