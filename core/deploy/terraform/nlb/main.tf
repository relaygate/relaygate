# Terraform: AWS Network Load Balancer for RelayGate dual active gateways
#
# 能力：
# - L4 TCP + UDP 端口透传（游戏入口 10001–10010 + canary 11001）
# - 五元组 / 源地址会话保持（stickiness）
# - 健康检查复用 Envoy /ready（HTTP）或 TCP 探测管理口
#
# 用法：
#   cd core/deploy/terraform/nlb
#   cp terraform.tfvars.example terraform.tfvars   # 填入真实值，勿提交密钥
#   terraform init
#   terraform plan
#   terraform apply   # 需显式审批；生产禁止无审批 apply
#
# 密钥：AWS 凭据走环境变量 / IAM Role / OIDC，不要写进本仓库。

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

locals {
  name_prefix = var.name_prefix
  # 游戏端口：生产 10001-10010 + canary 11001
  game_ports = concat(range(var.game_port_start, var.game_port_end + 1), [var.canary_port])
  gateway_map = { for g in var.gateways : g.name => g }
}

data "aws_vpc" "selected" {
  id = var.vpc_id
}

resource "aws_lb" "relaygate" {
  name               = "${local.name_prefix}-nlb"
  load_balancer_type = "network"
  internal           = false
  subnets            = var.public_subnet_ids

  enable_cross_zone_load_balancing = true

  tags = merge(var.tags, {
    Name    = "${local.name_prefix}-nlb"
    Product = "RelayGate"
  })
}

# TCP 目标组：HTTP 健康检查打 Envoy /ready（需管理口对 NLB 网段可达）
resource "aws_lb_target_group" "tcp" {
  for_each = toset([for p in local.game_ports : tostring(p)])

  name        = "${local.name_prefix}-tcp-${each.key}"
  port        = tonumber(each.key)
  protocol    = "TCP"
  vpc_id      = data.aws_vpc.selected.id
  target_type = "instance"

  # 源地址亲和：同一客户端 IP 粘滞到同一网关（接近五元组会话保持）
  stickiness {
    enabled = true
    type    = "source_ip"
  }

  health_check {
    enabled             = true
    protocol            = var.health_check_protocol
    port                = tostring(var.health_check_port)
    path                = var.health_check_protocol == "HTTP" ? var.health_check_path : null
    healthy_threshold   = 3
    unhealthy_threshold = 3
    interval            = 10
  }

  tags = merge(var.tags, {
    Protocol = "TCP"
    Port     = each.key
  })
}

# UDP 目标组：AWS 要求健康检查走 TCP（复用同一 /ready 端口）
resource "aws_lb_target_group" "udp" {
  for_each = toset([for p in local.game_ports : tostring(p)])

  name        = "${local.name_prefix}-udp-${each.key}"
  port        = tonumber(each.key)
  protocol    = "UDP"
  vpc_id      = data.aws_vpc.selected.id
  target_type = "instance"

  stickiness {
    enabled = true
    type    = "source_ip"
  }

  health_check {
    enabled             = true
    protocol            = "TCP"
    port                = tostring(var.health_check_port)
    healthy_threshold   = 3
    unhealthy_threshold = 3
    interval            = 10
  }

  tags = merge(var.tags, {
    Protocol = "UDP"
    Port     = each.key
  })
}

resource "aws_lb_target_group_attachment" "tcp" {
  for_each = {
    for pair in setproduct(keys(local.gateway_map), local.game_ports) :
    "${pair[0]}-tcp-${pair[1]}" => {
      gateway = pair[0]
      port    = tostring(pair[1])
    }
  }

  target_group_arn = aws_lb_target_group.tcp[each.value.port].arn
  target_id        = local.gateway_map[each.value.gateway].instance_id
  port             = tonumber(each.value.port)
}

resource "aws_lb_target_group_attachment" "udp" {
  for_each = {
    for pair in setproduct(keys(local.gateway_map), local.game_ports) :
    "${pair[0]}-udp-${pair[1]}" => {
      gateway = pair[0]
      port    = tostring(pair[1])
    }
  }

  target_group_arn = aws_lb_target_group.udp[each.value.port].arn
  target_id        = local.gateway_map[each.value.gateway].instance_id
  port             = tonumber(each.value.port)
}

resource "aws_lb_listener" "tcp" {
  for_each = aws_lb_target_group.tcp

  load_balancer_arn = aws_lb.relaygate.arn
  port              = tonumber(each.key)
  protocol          = "TCP"

  default_action {
    type             = "forward"
    target_group_arn = each.value.arn
  }
}

resource "aws_lb_listener" "udp" {
  for_each = aws_lb_target_group.udp

  load_balancer_arn = aws_lb.relaygate.arn
  port              = tonumber(each.key)
  protocol          = "UDP"

  default_action {
    type             = "forward"
    target_group_arn = each.value.arn
  }
}

# 可选：安全组规则说明用（NLB 本身无 SG；规则打在实例 SG 上）
resource "aws_security_group_rule" "allow_game_tcp_from_clients" {
  count = var.gateway_security_group_id == "" ? 0 : length(local.game_ports)

  type              = "ingress"
  security_group_id = var.gateway_security_group_id
  protocol          = "tcp"
  from_port         = local.game_ports[count.index]
  to_port           = local.game_ports[count.index]
  cidr_blocks       = var.client_cidrs
  description       = "RelayGate game TCP ${local.game_ports[count.index]}"
}

resource "aws_security_group_rule" "allow_game_udp_from_clients" {
  count = var.gateway_security_group_id == "" ? 0 : length(local.game_ports)

  type              = "ingress"
  security_group_id = var.gateway_security_group_id
  protocol          = "udp"
  from_port         = local.game_ports[count.index]
  to_port           = local.game_ports[count.index]
  cidr_blocks       = var.client_cidrs
  description       = "RelayGate game UDP ${local.game_ports[count.index]}"
}

resource "aws_security_group_rule" "allow_health_from_vpc" {
  count = var.gateway_security_group_id == "" ? 0 : 1

  type              = "ingress"
  security_group_id = var.gateway_security_group_id
  protocol          = "tcp"
  from_port         = var.health_check_port
  to_port           = var.health_check_port
  cidr_blocks       = [data.aws_vpc.selected.cidr_block]
  description       = "NLB health check to Envoy /ready"
}
