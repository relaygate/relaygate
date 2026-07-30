variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "ap-northeast-1"
}

variable "name_prefix" {
  description = "Resource name prefix"
  type        = string
  default     = "relaygate"
}

variable "vpc_id" {
  description = "VPC ID hosting gateway instances"
  type        = string
}

variable "public_subnet_ids" {
  description = "Public subnet IDs for internet-facing NLB (multi-AZ)"
  type        = list(string)
}

variable "gateways" {
  description = "Active-active gateway instances"
  type = list(object({
    name        = string
    instance_id = string
    private_ip  = string
  }))
}

variable "forward_port_start" {
  type    = number
  default = 10001
}

variable "forward_port_end" {
  type    = number
  default = 10010
}

variable "canary_port" {
  type    = number
  default = 11001
}

variable "health_check_protocol" {
  description = "TCP or HTTP (HTTP path uses Envoy /ready)"
  type        = string
  default     = "HTTP"
  validation {
    condition     = contains(["TCP", "HTTP"], var.health_check_protocol)
    error_message = "health_check_protocol must be TCP or HTTP"
  }
}

variable "health_check_port" {
  description = "Envoy admin / health port reachable from NLB (not public internet)"
  type        = number
  default     = 9901
}

variable "health_check_path" {
  type    = string
  default = "/ready"
}

variable "gateway_security_group_id" {
  description = "Optional: instance SG to attach ingress rules; empty to skip"
  type        = string
  default     = ""
}

variable "client_cidrs" {
  description = "Client CIDRs allowed to forward ports on instance SG"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "tags" {
  type    = map(string)
  default = {}
}

variable "enable_proxy_protocol_v2" {
  description = <<-EOT
    TCP target group: send PROXY v2 to targets. Default false.
    Gateway product default is PROXY_PROTOCOL=off (public direct exposure).
    Set true only with gateway PROXY_PROTOCOL=v2 (or v2-compat) and SG locking
    forward ports to the NLB — never on publicly reachable listeners.
    See docs/logging-playbook.md. Not applied to UDP target groups.
  EOT
  type    = bool
  default = false
}
