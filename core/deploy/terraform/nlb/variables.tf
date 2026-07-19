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

variable "game_port_start" {
  type    = number
  default = 10001
}

variable "game_port_end" {
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
  description = "Client CIDRs allowed to game ports on instance SG"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "tags" {
  type    = map(string)
  default = {}
}
