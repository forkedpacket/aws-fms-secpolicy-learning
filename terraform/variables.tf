variable "aws_region" {
  description = "AWS region to deploy demo infra and FMS policies"
  type        = string
  default     = "us-west-2"
}

variable "lambda_package" {
  description = "Path to the built Lambda ZIP (e.g., dist/lambda.zip)"
  type        = string
  default     = "../dist/lambda.zip"
}

variable "lambda_timeout" {
  description = "Lambda timeout in seconds"
  type        = number
  default     = 30
}

variable "lambda_memory" {
  description = "Lambda memory in MB"
  type        = number
  default     = 256
}

variable "ou_id" {
  description = "Target OU for discovery (optional). Leave empty to disable OU filtering."
  type        = string
  default     = ""
}

variable "primary_tag_key" {
  description = "Tag key used to choose the primary rule set"
  type        = string
  default     = "WafRulesetPrimary"
}

variable "secondary_tag_key" {
  description = "Tag key used to choose the secondary rule set"
  type        = string
  default     = "WafRulesetSecondary"
}

variable "default_primary_rules" {
  description = "Default primary rule set name when the tag is missing or unknown"
  type        = string
  default     = "ou-shared-edge"
}

variable "default_secondary_rules" {
  description = "Default secondary rule set name when the tag is missing or unknown"
  type        = string
  default     = "ou-shared-bot"
}

variable "config_ssm_param" {
  description = "SSM parameter name that stores the rendered policy variants YAML"
  type        = string
  default     = "/aws-fms-secpolicy/policy-variants"
}
