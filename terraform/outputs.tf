output "alb_dns_name" {
  description = "DNS name of the demo ALB"
  value       = aws_lb.demo.dns_name
}

output "alb_arn" {
  description = "ARN of the demo ALB (no WebACL attached by default)"
  value       = aws_lb.demo.arn
}

output "lambda_name" {
  description = "Name of the Lambda renderer"
  value       = aws_lambda_function.renderer.function_name
}

output "primary_rule_group_arn" {
  description = "ARN of the primary OU-like rule group created in this account"
  value       = aws_wafv2_rule_group.primary_ou_shared_edge.arn
}

output "secondary_rule_group_arn" {
  description = "ARN of the secondary OU-like rule group created in this account"
  value       = aws_wafv2_rule_group.secondary_ou_shared_bot.arn
}
