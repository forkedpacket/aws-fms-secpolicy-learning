````markdown
# aws-fms-secpolicy-learning

Lambda-driven **AWS Firewall Manager (FMS) + WAFv2** renderer that discovers OU-scoped, WAF-attached resources and attaches rule groups based on tags. Terraform boots a baseline ALB **without** a WebACL, creates demo WAF rule groups, publishes the config to SSM, and deploys the Lambda/IAM plumbing; the Lambda renders `managed_service_data` and updates FMS policies inside the admin account.

Free-tier note: Application Load Balancers are **not free-tier**. Keep runs short and destroy when done.

---

## Architecture

```mermaid
flowchart LR
    A["AWS Org"] -->|delegates| B["FMS Admin Account"]
    B -->|Lambda env: OU_ID + tag keys + rule ARNs (SSM)| C["Lambda WAF Renderer"]
    D["Terraform Demo Infra"] -->|provisions| E["ALB (no WAF) + demo WAF RGs"]
    C -->|discovers| E
    C -->|list WebACL resources<br/>read tags| F["Resource Inventory"]
    C -->|select rule groups by tags| G["Policy Models"]
    G -->|render managed_service_data| H["FMS Policy Update"]
    H -->|enforces WAFv2| E
```

---

## What’s Included

- **Terraform**: VPC + ALB with **no WebACL**, demo WAF rule groups, IAM roles/policies, SSM parameter with the policy-variants YAML, and a Lambda skeleton (packaging hook left to you).
- **Lambda flow**: Discover WAF-attached resources in a target OU → read tags → select primary/secondary rule groups (managed identifiers or ARNs) → render `managed_service_data` → `PutPolicy` back to FMS.
- **Config**: `configs/policy-variants.yaml` maps tag values to rule groups (ARNs) and defaults; Terraform also renders `terraform/policy-variants.tpl.yaml` into SSM.
- **Templates**: `templates/fms_policy.tmpl` shapes the WAFv2 JSON used by FMS.
- **Local harness**: `cmd/renderer` mirrors the Lambda logic for dry runs/tests.

---

## Quickstart (demo)

1) **Build Lambda ZIP**

```bash
mkdir -p dist/lambda
GOOS=linux GOARCH=amd64 go build -o dist/lambda/main ./cmd/lambda
cd dist/lambda && zip ../lambda.zip main && cd ../..
```

2) **Boot demo infra**

```bash
cd terraform
terraform init
terraform apply -auto-approve
```

Creates: VPC + public ALB (no WAF/WebACL), IAM roles, demo WAF rule groups, SSM parameter with policy variants, and Lambda scaffolding. Capture outputs for ALB ARN and Lambda name.

3) **Configure/Run the Lambda**

Env vars (set via Terraform):

- `OU_ID` – Organizational Unit to scope discovery (empty to skip check).
- `PRIMARY_TAG_KEY` / `SECONDARY_TAG_KEY` – defaults `WafRulesetPrimary` / `WafRulesetSecondary`.
- `DEFAULT_PRIMARY_RULES` / `DEFAULT_SECONDARY_RULES` – default rule set names from config.
- `CONFIG_SSM_PARAM` – SSM parameter name with the YAML (populated by Terraform).

Trigger a test event:

- `{ "dryRun": true }` to log intended changes.
- `{ "dryRun": false }` (or omit) to apply via `fms:PutPolicy`.

Local dry-runs: `go run ./cmd/renderer -input resources.json -config configs/policy-variants.yaml`.

4) **Verify**

- Check FMS console for updated policies (`auto-alb-*`).
- Confirm ALB shows the expected WAFv2 WebACL/rules.

5) **Teardown**

```bash
cd terraform
terraform destroy
```

---

## IAM Permissions (Lambda role)

- Discovery: `wafv2:ListResourcesForWebACL`, `wafv2:GetWebACLForResource`, `elasticloadbalancing:DescribeLoadBalancers`, `elasticloadbalancing:DescribeTags`.
- Org scoping: `organizations:ListAccounts`, `organizations:ListChildren`, `organizations:DescribeOrganization`, `organizations:DescribeOrganizationalUnit`.
- Policy updates: `fms:PutPolicy`, `fms:GetPolicy`, `fms:ListPolicies`.
- Config: `ssm:GetParameter` for the variants YAML parameter.
- Logging/ops: `logs:CreateLogGroup`, `logs:CreateLogStream`, `logs:PutLogEvents`.
- Demo infra (Terraform user): VPC/ALB create permissions plus `iam:PassRole` for Lambda.

Tighten to least-privilege for production.

---

## How it Works

1. **Discovery**: List resources with WebACLs within the target OU, normalize ARNs/tags.
2. **Selection**: Read two tag keys (primary/secondary). Look up rule groups in `configs/policy-variants.yaml`; apply defaults if tags absent.
3. **Render**: Use `templates/fms_policy.tmpl` to build WAFv2 `managed_service_data` JSON.
4. **Apply**: `fms:PutPolicy` attaches/updates the policy for the resource’s scope.
5. **Safety**: Provide a dry-run flag to log intended attachments without calling FMS.

---

## Config & Tags

- Tags expect values that map to entries in `configs/policy-variants.yaml` (managed identifiers or ARNs). Terraform also materializes `terraform/policy-variants.tpl.yaml` with demo rule group ARNs into SSM.
- Example (demo defaults):

```yaml
tagKeys:
  primary: WafRulesetPrimary
  secondary: WafRulesetSecondary
ruleSets:
  primary:
    ou-shared-edge:
      ruleGroups:
        - arn: arn:aws:wafv2:us-west-2:...:regional/rulegroup/ou-shared-edge/...
  secondary:
    ou-shared-bot:
      ruleGroups:
        - arn: arn:aws:wafv2:us-west-2:...:regional/rulegroup/ou-shared-bot/...
defaults:
  primary: ou-shared-edge
  secondary: ou-shared-bot
```

Missing tags fall back to the `defaults` entries.

---

## Development Loop

1. Edit config/template or policy logic.
2. `go test ./...` for unit coverage on discovery/selection/rendering.
3. `go run ./cmd/renderer -input resources.json ...` for local dry runs.
4. Rebuild/deploy Lambda; re-run Terraform if IAM/env vars or demo rule groups/config change.

---

## Key Files

- `cmd/renderer/main.go` – Local entrypoint mirroring the Lambda logic.
- `cmd/lambda/main.go` – Lambda entrypoint (OU check, SSM config load, discovery, FMS).
- `internal/discovery` – Discovery utilities (ALBs) + OU membership check.
- `internal/policy/policy.go` – Tag-based rule selection and `managed_service_data` render.
- `internal/fmsapply/fmsapply.go` – FMS PutPolicy helper.
- `configs/policy-variants.yaml` – Tag → rule-group mapping (managed or ARN).
- `terraform/policy-variants.tpl.yaml` – Template used to publish config to SSM.
- `templates/fms_policy.tmpl` – WAFv2 JSON template.
- `terraform/main.tf` – ALB (no WAF) + IAM + Lambda + demo rule groups + SSM.
- `demo.sh` – Optional local orchestration.

---

## Suggested Enhancements

1. Add CloudFront/API Gateway discovery paths.
2. Add rate-based or geo match statements to the template.
3. Add OU/account allow/deny lists per policy.
4. Emit human-friendly reports (table of resources → rule set decisions).
````
