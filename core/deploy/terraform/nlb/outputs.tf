output "nlb_dns_name" {
  description = "Players should connect to this DNS (or alias) instead of a single gateway IP"
  value       = aws_lb.relaygate.dns_name
}

output "nlb_arn" {
  value = aws_lb.relaygate.arn
}

output "tcp_target_group_arns" {
  value = { for k, tg in aws_lb_target_group.tcp : k => tg.arn }
}

output "udp_target_group_arns" {
  value = { for k, tg in aws_lb_target_group.udp : k => tg.arn }
}

output "health_check" {
  value = {
    protocol = var.health_check_protocol
    port     = var.health_check_port
    path     = var.health_check_path
  }
}

output "backend_allowlist_reminder" {
  description = "Game servers must allow BOTH gateway private/public source IPs"
  value       = [for g in var.gateways : "${g.name}=${g.private_ip}"]
}
