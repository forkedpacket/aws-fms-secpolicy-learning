# AGENTS: aws-fms-secpolicy-learning

This repo is designed to be extended by AI agents (ChatGPT, Grok, Copilot, etc.).

## Mission

Demonstrate a **Lambda-driven** AWS Firewall Manager (FMS) + WAFv2 renderer that:

- Runs inside the FMS admin account.
- Discovers **WAF-attached resources within a target OU**.
- Uses **two tags** to decide which **rule groups** to attach (e.g., `WafRulesetPrimary`, `WafRulesetSecondary`).
- Renders `managed_service_data` JSON and applies it back through FMS.
- Ships with Terraform that creates a baseline **ALB without a WAF** plus IAM/Lambda scaffolding.

## Key Files

- `cmd/renderer/main.go` – Local render harness (mirrors Lambda logic).
- `cmd/lambda/main.go` – Lambda entrypoint with OU check, SSM config load, discovery, and FMS apply.
- `internal/discovery/...` – Discovery helpers (ALBs today) + OU membership check.
- `internal/policy/policy.go` – Builds FMS/WAF policy models + renders `managed_service_data` (managed or ARN rule groups).
- `internal/fmsapply/fmsapply.go` – Upserts FMS policies.
- `configs/policy-variants.yaml` – Tag → rule-group mapping (managed identifiers or ARNs) and defaults.
- `templates/fms_policy.tmpl` – JSON template for WAFv2 `managed_service_data`.
- `terraform/main.tf` – Baseline ALB (no WAF), IAM, Lambda, local WAF rule groups, and SSM parameter for config.
- `terraform/policy-variants.tpl.yaml` – Template used to publish config into SSM.
- `demo.sh` – Optional local build/apply helper (builds Lambda zip, applies Terraform).

## Style Guidelines

- Use Go 1.23 conventions/toolchain (module pins Go 1.23).
- Keep dependencies minimal (stdlib + `aws-sdk-go-v2` + `gopkg.in/yaml.v3`).
- Write small, focused functions.
- Favor composition over inheritance-like patterns.
- Add unit tests in `internal/...` and `tests/` when modifying behavior.

## Suggested AI Tasks

1. **Lambda + OU discovery**
   - Enumerate WAF-attached resources in a given OU.
   - Normalize their tags and feed them to the policy selector.

2. **Tag-driven rule selection**
   - Support two tag keys (primary/secondary) to pick rule groups.
   - Add defaults when either tag is missing.

3. **Terraform bootstrap**
   - Keep demo ALB **without a WAF**, IAM, Lambda packaging.
   - Maintain local WAF rule groups + SSM publishing so Lambda consumes ARNs automatically.

4. **Safety + verification**
   - Add unit tests around discovery and rule set merging.
   - Provide a dry-run mode (log planned attachments, no writes).

When in doubt, prefer **small incremental changes** with comments that explain *why*, not just *what*.
