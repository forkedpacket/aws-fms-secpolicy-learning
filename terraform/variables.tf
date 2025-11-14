variable "aws_region" {
  description = "AWS region to deploy demo infra and FMS policies"
  type        = string
  default     = "us-west-2"
}

variable "fms_policies_json" {
  description = "Map of rendered FMS policies from Go (name -> object)"
  type        = map(any)
  default     = {}
}
