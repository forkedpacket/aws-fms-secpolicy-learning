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
    env         = "prod"
    visibility  = "public"
    sensitivity = "Dhigh"
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
# FMS / WAFv2 policy application
# -----------------------------------------------------------------------------

# Expect fms_policies_json to look like:
# {
#   "auto-alb-...": {
#     "name": "auto-alb-...",
#     "description": "...",
#     "resource_type": "AWS::ElasticLoadBalancingV2::LoadBalancer",
#     "scope": "REGIONAL",
#     "managed_service_data": "{... JSON ...}"
#   }
# }

locals {
  fms_policies = var.fms_policies_json
}

# Optional module stub: FMS admin setup (placeholder for real org-level setup).
module "fms_setup" {
  source = "./modules/fms-setup"

  # In a real-world scenario, you would pass org/OU/account info here.
  create_placeholder = true
}

resource "aws_fms_policy" "waf_v2" {
  for_each = local.fms_policies

  name                  = each.value.name
  delete_all_policy_resources = false
  exclude_resource_tags       = false
  remediation_enabled         = true

  # Using resource_type_list for ALBs.
  resource_type_list = [each.value.resource_type]

  # Include all accounts in the org. For a PoC, we omit include_map/exclude_map
  # and rely on tagging / resource types to scope application.

  security_service_policy_data {
    type                 = "WAFV2"
    managed_service_data = each.value.managed_service_data
  }

  depends_on = [module.fms_setup]
}
