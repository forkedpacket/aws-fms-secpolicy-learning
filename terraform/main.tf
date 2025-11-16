locals {
  project_name = "aws-fms-secpolicy-learning"
}

# -----------------------------------------------------------------------------
# Demo networking + ALB
# -----------------------------------------------------------------------------

resource "aws_vpc" "demo" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "${local.project_name}-vpc"
  }
}

resource "aws_internet_gateway" "demo" {
  vpc_id = aws_vpc.demo.id

  tags = {
    Name = "${local.project_name}-igw"
  }
}

resource "aws_subnet" "demo_public_a" {
  vpc_id                  = aws_vpc.demo.id
  cidr_block              = "10.0.0.0/24"
  availability_zone       = "${var.aws_region}a"
  map_public_ip_on_launch = true

  tags = {
    Name = "${local.project_name}-public-a"
  }
}

resource "aws_subnet" "demo_public_b" {
  vpc_id                  = aws_vpc.demo.id
  cidr_block              = "10.0.1.0/24"
  availability_zone       = "${var.aws_region}b"
  map_public_ip_on_launch = true

  tags = {
    Name = "${local.project_name}-public-b"
  }
}

resource "aws_route_table" "demo_public" {
  vpc_id = aws_vpc.demo.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.demo.id
  }

  tags = {
    Name = "${local.project_name}-public-rt"
  }
}

resource "aws_route_table_association" "demo_public_a" {
  subnet_id      = aws_subnet.demo_public_a.id
  route_table_id = aws_route_table.demo_public.id
}

resource "aws_route_table_association" "demo_public_b" {
  subnet_id      = aws_subnet.demo_public_b.id
  route_table_id = aws_route_table.demo_public.id
}

resource "aws_security_group" "demo_alb" {
  name        = "${local.project_name}-alb-sg"
  description = "Allow HTTP"
  vpc_id      = aws_vpc.demo.id

  ingress {
    description = "HTTP from anywhere"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${local.project_name}-alb-sg"
  }
}

resource "aws_lb" "demo" {
  name               = "${local.project_name}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.demo_alb.id]
  subnets            = [aws_subnet.demo_public_a.id, aws_subnet.demo_public_b.id]

  tags = {
    Name        = "${local.project_name}-alb"
    WafRulesetPrimary   = "ou-shared-edge"
    WafRulesetSecondary = "ou-shared-bot"
  }
}

resource "aws_lb_target_group" "demo" {
  name     = "${local.project_name}-tg"
  port     = 80
  protocol = "HTTP"
  vpc_id   = aws_vpc.demo.id

  health_check {
    path = "/"
  }

  tags = {
    Name = "${local.project_name}-tg"
  }
}

resource "aws_lb_listener" "demo_http" {
  load_balancer_arn = aws_lb.demo.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "fixed-response"

    fixed_response {
      content_type = "text/plain"
      message_body = "Hello from aws-fms-secpolicy-learning demo"
      status_code  = "200"
    }
  }
}

# -----------------------------------------------------------------------------
# Local rule groups to mimic OU-managed sets
# -----------------------------------------------------------------------------

resource "aws_wafv2_rule_group" "primary_ou_shared_edge" {
  name     = "${local.project_name}-primary-edge"
  scope    = "REGIONAL"
  capacity = 50

  rule {
    name     = "BlockAdminPaths"
    priority = 1
    action {
      block {}
    }
    statement {
      byte_match_statement {
        positional_constraint = "CONTAINS"
        search_string         = "/admin"
        field_to_match {
          uri_path {}
        }
        text_transformation {
          priority = 0
          type     = "NONE"
        }
      }
    }
    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${local.project_name}-primary-admin-paths"
      sampled_requests_enabled   = true
    }
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = "${local.project_name}-primary"
    sampled_requests_enabled   = true
  }
}

resource "aws_wafv2_rule_group" "secondary_ou_shared_bot" {
  name     = "${local.project_name}-secondary-bot"
  scope    = "REGIONAL"
  capacity = 50

  rule {
    name     = "ChallengeUnknownAgents"
    priority = 1
    action {
      challenge {}
    }
    statement {
      byte_match_statement {
        positional_constraint = "CONTAINS"
        search_string         = "curl"
        field_to_match {
          single_header {
            name = "user-agent"
          }
        }
        text_transformation {
          priority = 0
          type     = "LOWERCASE"
        }
      }
    }
    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${local.project_name}-secondary-bots"
      sampled_requests_enabled   = true
    }
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = "${local.project_name}-secondary"
    sampled_requests_enabled   = true
  }
}

locals {
  policy_variants_yaml = templatefile("${path.module}/policy-variants.tpl.yaml", {
    primary_tag_key   = var.primary_tag_key
    secondary_tag_key = var.secondary_tag_key
    primary_arn       = aws_wafv2_rule_group.primary_ou_shared_edge.arn
    secondary_arn     = aws_wafv2_rule_group.secondary_ou_shared_bot.arn
  })
}

resource "aws_ssm_parameter" "policy_variants" {
  name        = var.config_ssm_param
  description = "FMS/WAF policy variants for Lambda renderer"
  type        = "String"
  value       = local.policy_variants_yaml
}

# -----------------------------------------------------------------------------
# Lambda: tag-driven WAF policy renderer (runs inside FMS admin account)
# -----------------------------------------------------------------------------

data "aws_iam_policy_document" "lambda_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "lambda" {
  name               = "${local.project_name}-lambda-role"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

data "aws_iam_policy_document" "lambda_inline" {
  statement {
    sid    = "FMSPolicyUpdates"
    effect = "Allow"
    actions = [
      "fms:ListPolicies",
      "fms:GetPolicy",
      "fms:PutPolicy"
    ]
    resources = ["*"]
  }

  statement {
    sid    = "Discovery"
    effect = "Allow"
    actions = [
      "elasticloadbalancing:DescribeLoadBalancers",
      "elasticloadbalancing:DescribeTags",
      "organizations:ListAccountsForParent",
      "sts:GetCallerIdentity"
    ]
    resources = ["*"]
  }

  statement {
    sid    = "Logging"
    effect = "Allow"
    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents"
    ]
    resources = ["*"]
  }

  statement {
    sid    = "ConfigFromSSM"
    effect = "Allow"
    actions = [
      "ssm:GetParameter"
    ]
    resources = [aws_ssm_parameter.policy_variants.arn]
  }
}

resource "aws_iam_role_policy" "lambda" {
  name   = "${local.project_name}-lambda-inline"
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.lambda_inline.json
}

resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${local.project_name}-renderer"
  retention_in_days = 14
}

resource "aws_lambda_function" "renderer" {
  function_name = "${local.project_name}-renderer"
  description   = "OU-scoped tag-driven WAF policy renderer"
  role          = aws_iam_role.lambda.arn
  handler       = "main"
  runtime       = "provided.al2"

  filename         = var.lambda_package
  source_code_hash = filebase64sha256(var.lambda_package)

  timeout = var.lambda_timeout
  memory_size = var.lambda_memory

  environment {
    variables = {
      OU_ID                  = var.ou_id
      PRIMARY_TAG_KEY        = var.primary_tag_key
      SECONDARY_TAG_KEY      = var.secondary_tag_key
      DEFAULT_PRIMARY_RULES  = var.default_primary_rules
      DEFAULT_SECONDARY_RULES= var.default_secondary_rules
      CONFIG_SSM_PARAM       = var.config_ssm_param
    }
  }

  depends_on = [aws_iam_role_policy.lambda]
}
