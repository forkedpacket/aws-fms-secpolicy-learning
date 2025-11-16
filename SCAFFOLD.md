````markdown
# aws-fms-secpolicy-learning – Scaffold

Scaffold for a Lambda that renders **FMS/WAFv2 `managed_service_data`** in the FMS admin account. The Lambda scans **WAF-attached resources in a target OU**, reads two tag keys, and attaches rule groups accordingly. Terraform provides a demo ALB **without a WebACL**, demo WAF rule groups, IAM/Lambda wiring, and publishes the policy config to SSM.

---

## Architecture to Keep In Mind

1. **Inputs**: OU ID, tag keys (`PRIMARY_TAG_KEY`, `SECONDARY_TAG_KEY`), default rule set names, and an SSM param name for the YAML config; template paths.
2. **Discovery**: List resources with WebACLs scoped to the OU; normalize ARNs/tags.
3. **Selection**: Map tag values to rule groups via `configs/policy-variants.yaml`, with sensible defaults.
4. **Render**: Fill `templates/fms_policy.tmpl` to produce `managed_service_data`.
5. **Apply**: `fms:PutPolicy` to update/attach policies; dry-run bypasses writes.

---

## Terraform Scaffold

- Baseline ALB **without WAF** to demonstrate attachment changes.
- Demo WAF rule groups (primary/secondary) to mimic OU-managed sets.
- SSM parameter populated from `terraform/policy-variants.tpl.yaml` with the rule group ARNs.
- IAM for Lambda: Organizations reads, WAFv2 resource listing, FMS policy updates, SSM read, CloudWatch Logs.
- Lambda packaging hook is intentionally minimal; add your build/zip/sam step before `terraform apply` or use an external deploy.
- Variables for `ou_id`, tag keys, defaults, and config param name so the Lambda can stay generic.

---

## Extending the Policy Logic

- **Add/adjust rule sets**: Edit `configs/policy-variants.yaml` to map tag values to rule group ARNs; update `defaults` for missing tags.
- **Template changes**: Modify `templates/fms_policy.tmpl` for new statements (rate-based, geo, header/path matches). Keep JSON valid.
- **New resource types**: Extend `internal/discovery` to enumerate CloudFront/API GW, normalize resources, and feed them into `policy` package.
- **Safety**: Keep a dry-run toggle that logs planned attachments without calling FMS.

---

## Development Loop

1. Edit config/template or Go logic.
2. `go test ./...` to cover discovery, selection, and rendering.
3. `go run ./cmd/renderer ...` for local dry runs against `resources.json`.
4. Rebuild/deploy the Lambda; re-run Terraform if IAM/env/config or demo rule groups change.

---

## Notes for AI Agents

- Prefer small, composable functions using stdlib + `aws-sdk-go-v2` + `gopkg.in/yaml.v3`.
- Add unit tests when changing behavior; keep dependencies lean.
- Document why changes are made, especially around rule selection, SSM config, and OU scoping.
````
