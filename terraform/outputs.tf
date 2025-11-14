output "alb_dns_name" {
  description = "DNS name of the demo ALB"
  value       = aws_lb.demo.dns_name
}

output "fms_policy_arns" {
  description = "ARNs of created FMS policies"
  value       = [for p in aws_fms_policy.waf_v2 : p.arn]
}
