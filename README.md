# aws-fms-secpolicy-learning

Lambda-driven, tag-based AWS Firewall Manager (FMS) + WAFv2 lab. Terraform creates a demo ALB **without** a WebACL plus IAM/Lambda scaffolding. The Lambda discovers WAF-attachable resources in a target OU, reads two tag keys, selects rule groups, renders `managed_service_data`, and upserts FMS policies.

---

## What It Does

- **Discovery**: Enumerates ALBs in the account; optional OU check blocks execution if the caller is outside the target OU.
- **Selection**: Reads two tag keys (`WafRulesetPrimary`, `WafRulesetSecondary` by default) and picks rule groups from `configs/policy-variants.yaml`, with defaults when tags are missing.
- **Rendering**: Uses `templates/fms_policy.tmpl` to build WAFv2 `managed_service_data`.
- **Apply**: Calls `fms:PutPolicy` to create/update policies in the FMS admin account. A `dryRun` flag logs actions only.
- **Local harness**: `cmd/renderer` mirrors the Lambda logic for dry runs and tests.
-- **Config**: Terraform writes an SSM parameter containing the policy-variants YAML with the locally created rule group ARNs; Lambda loads it via `CONFIG_SSM_PARAM`.

---

## Architecture

```mermaid
flowchart LR
    A[AWS Org] -->|target OU| B[FMS Admin Account]
    B -->|terraform| C[ALB (no WAF) + IAM + Lambda]
    D[Lambda Event] --> C
    C -->|discover ALBs| E[Resource Inventory]
    E -->|tags primary/secondary| F[Rule Set Selection]
    F -->|render template| G[managed_service_data JSON]
    G -->|fms:PutPolicy| H[FMS Policies]
    H -->|enforce WAFv2| C
```

---

## Repo Layout

```
cmd/renderer          # CLI for local dry runs
cmd/lambda            # Lambda entrypoint (uses same policy logic)
configs/policy-variants.yaml  # Tag -> rule set mapping
configs/embed.go      # Embedded config for Lambda packaging
internal/
  config/             # YAML schema + validation
  discovery/          # ALB discovery + OU membership check
  fmsapply/           # FMS PutPolicy helper
  policy/             # Rule selection + managed_service_data rendering
  util/               # Logger
templates/fms_policy.tmpl
terraform/            # Demo VPC + ALB (no WAF) + IAM + Lambda stub
resources.json        # Sample resource input for offline rendering
```

Supporting docs: `AGENTS.md`, `BOTREADME.md`, `SCAFFOLD.md`.

---

## Prereqs

- AWS Organization with a delegated **FMS admin account**.
- Permissions for Lambda role: `fms:ListPolicies/GetPolicy/PutPolicy`, `elasticloadbalancing:Describe*`, `organizations:ListAccountsForParent`, `sts:GetCallerIdentity`, CloudWatch Logs.
- Local tools: Go 1.22+, Terraform 1.5+, AWS CLI v2.

Quick checks:

```bash
aws sts get-caller-identity
go version
terraform version
```

---

## Quickstart

1) **Build the Lambda ZIP**

```bash
mkdir -p dist/lambda
GOOS=linux GOARCH=amd64 go build -o dist/lambda/main ./cmd/lambda
cd dist/lambda && zip ../lambda.zip main && cd ../..
```

2) **Terraform apply (creates VPC + ALB w/o WAF + IAM + Lambda + WAF rule groups)**

```bash
cd terraform
terraform init
terraform apply -auto-approve
```

Important inputs (environment variables set on the Lambda):

- `OU_ID` – OU to scope membership (empty to disable check).
- `PRIMARY_TAG_KEY` / `SECONDARY_TAG_KEY` – default `WafRulesetPrimary` / `WafRulesetSecondary`.
- `DEFAULT_PRIMARY_RULES` / `DEFAULT_SECONDARY_RULES` – default rule set names from `configs/policy-variants.yaml`.
- `CONFIG_SSM_PARAM` – SSM parameter containing the YAML with actual rule group ARNs (Terraform populates this).

3) **Invoke the Lambda**

- Trigger a test event like `{ "dryRun": true }` to log intended FMS changes.
- Use `{ "dryRun": false }` (or omit) to apply via `fms:PutPolicy`.

4) **Verify**

- Check FMS console for policies named `auto-alb-*`.
- Confirm the ALB now has the expected WAFv2 WebACL/rules.

5) **Destroy**

```bash
cd terraform
terraform destroy
```

---

## Policy Config (tags → rule sets)

`configs/policy-variants.yaml` defines:

- `resourceDefaults.alb` – base (managed) rule groups applied to all ALBs.
- `tagKeys.primary/secondary` – tag names to read.
- `ruleSets.primary/secondary` – **rule group ARNs or managed identifiers** keyed by tag value. Use ARNs for OU-managed rule groups; vendor/name for AWS-managed ones.
- `defaults.primary/secondary` – fallback rule set names if tags are missing/invalid.

Example tags for the demo ALB:

```
WafRulesetPrimary   = "ou-shared-edge"
WafRulesetSecondary = "ou-shared-bot"
```

---

## Local Dry Run

```bash
go run ./cmd/renderer \
  -input resources.json \
  -config configs/policy-variants.yaml \
  -output generated/policies.json
```

This uses the same rule-selection and template logic as the Lambda.

---

## Tests

```bash
go test ./...
```

Unit tests cover rule-set selection and JSON rendering.

---

## Notes

- The ALB is intentionally created **without** a WebACL so Lambda-applied FMS policies are visible.
- OU filtering is a guardrail; discovery only runs if the current account is inside the target OU.
- Keep the Lambda package path (`var.lambda_package`) in sync with your build output (`dist/lambda.zip` by default).
