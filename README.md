````markdown
# aws-fms-secpolicy-learning

Dynamic **AWS Firewall Manager (FMS) + WAFv2** policy rendering PoC, driven by **Go** and applied via **Terraform**.

This repo demonstrates:

- Discovering **WAF-attachable resources** (ALBs) via AWS SDK for Go v2.
- Mapping **tags + resource types** → **FMS/WAF policy variants** (prod vs public vs sensitivity).
- Rendering **FMS `managed_service_data` JSON** using `text/template` with validation.
- Applying the rendered policies via **Terraform** (`aws_fms_policy`).
- A **demo script** that orchestrates discovery → render → terraform apply.

Free-tier note: Application Load Balancers **are not free-tier** and accrue per-hour and LCU charges. Keep this PoC short-lived and always run `terraform destroy` when done.

---

## Architecture

```mermaid
flowchart LR
    A[AWS Org + FMS Admin Account] -->|pre-req| B[Terraform Demo Infra]
    B -->|provisions| C[Tagged ALB (env=prod, sensitivity=Dhigh)]
    C -->|AWS SDK v2<br/>ALB + Tags| D[Go Renderer CLI]

    D -->|reads| E[policy-variants.yaml]
    D -->|renders via text/template| F[policies.json]

    F -->|wrapped as tfvars| G[Terraform FMS]
    G -->|aws_fms_policy| H[AWS Firewall Manager]

    H -->|enforces WAFv2<br/>policies| C
````

* Terraform creates a **demo VPC + ALB** with tags.
* Go **discovers ALBs**, reads **config** and **templates**, and emits a `policies.json` map.
* Terraform consumes that JSON as input and creates **FMS policies** that attach WAFv2 managed rule groups to matched resources.

---

## Repository Layout

```text
aws-fms-secpolicy-learning/
  README.md
  AGENTS.md

  go.mod

  cmd/
    renderer/
      main.go

  internal/
    config/
      config.go
    discovery/
      discovery.go
    policy/
      policy.go
    util/
      logging.go

  configs/
    policy-variants.yaml

  templates/
    fms_policy.tmpl

  terraform/
    provider.tf
    main.tf
    variables.tf
    outputs.tf
    modules/
      fms-setup/
        main.tf
        variables.tf
        outputs.tf

  demo.sh
  resources.json         # sample/resources-only mode
```

---

## Prerequisites

### AWS-side

You need:

1. An AWS Organization (management account).
2. A **delegated Firewall Manager admin account** (you’ll run this PoC from there).
3. FMS enabled in that admin account.

High-level steps (do these once in your AWS Org):

1. Enable **AWS Organizations**.
2. In the **Org management account**, delegate FMS admin account.
3. Log into the **FMS admin account** and confirm FMS is usable.

### Local tooling

* **Go** 1.21+
* **Terraform** 1.5+
* **AWS CLI** v2 (optional, but handy)
* AWS credentials configured (via `~/.aws/credentials`, SSO, or environment).

Verify:

```bash
aws sts get-caller-identity
go version
terraform version
```

---

## IAM Permissions

The IAM principal you use for the PoC needs:

1. For **discovery** (ALBs + tags):

   * `elasticloadbalancing:DescribeLoadBalancers`
   * `elasticloadbalancing:DescribeTags`

2. For **Firewall Manager / WAF**:

   * `fms:PutPolicy`
   * `fms:GetPolicy`
   * `fms:ListPolicies`
   * `fms:DeletePolicy`

3. For **Terraform demo infra**:

   * Typical VPC + ALB + SG permissions:

     * `ec2:*` (or a more restricted subset: `Describe*`, `CreateVpc`, `CreateSubnet`, `CreateSecurityGroup`, `AuthorizeSecurityGroupIngress`, `CreateInternetGateway`, `AttachInternetGateway`, `CreateRouteTable`, `AssociateRouteTable`, `CreateRoute`, etc.)
     * `elasticloadbalancing:*` (or narrowed to `CreateLoadBalancer`, `CreateTargetGroup`, `RegisterTargets`, `Describe*`, etc.)

Minimal example IAM policy **snippet** for the PoC principal (attach to a role/user):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "FMSCore",
      "Effect": "Allow",
      "Action": [
        "fms:PutPolicy",
        "fms:GetPolicy",
        "fms:ListPolicies",
        "fms:DeletePolicy"
      ],
      "Resource": "*"
    },
    {
      "Sid": "DiscoveryALB",
      "Effect": "Allow",
      "Action": [
        "elasticloadbalancing:DescribeLoadBalancers",
        "elasticloadbalancing:DescribeTags"
      ],
      "Resource": "*"
    },
    {
      "Sid": "DemoInfra",
      "Effect": "Allow",
      "Action": [
        "ec2:*",
        "elasticloadbalancing:*",
        "iam:PassRole"
      ],
      "Resource": "*"
    }
  ]
}
```

Tighten `ec2:*` / `elasticloadbalancing:*` in real environments.

---

## Quickstart

### 1. Clone and bootstrap

```bash
git clone https://github.com/your-org/aws-fms-secpolicy-learning.git
cd aws-fms-secpolicy-learning

go mod tidy
```

### 2. Create demo infra (VPC + ALB)

From the repo root:

```bash
cd terraform
terraform init

# Optional: see what will be created
terraform plan

# Create infra (VPC, subnets, SG, ALB with tags)
terraform apply -auto-approve
cd ..
```

This creates an **internet-facing ALB** tagged with:

* `env = "prod"`
* `visibility = "public"`
* `sensitivity = "Dhigh"`

### 3. Discover and render FMS/WAF policies

```bash
# Discover resources in AWS and render FMS managed_service_data policies
go run ./cmd/renderer \
  -discover \
  -region us-west-2 \
  -config configs/policy-variants.yaml \
  -output generated/policies.json
```

This will:

* Use AWS SDK v2 to **list ALBs** + tags.
* Match them against `configs/policy-variants.yaml`.
* Render JSON policy objects into `generated/policies.json`.

Example (simplified) structure:

```json
{
  "prod-alb-waf-strict": {
    "name": "prod-alb-waf-strict",
    "description": "Auto-generated WAFv2 policy for env=prod ALBs",
    "resource_type": "AWS::ElasticLoadBalancingV2::LoadBalancer",
    "managed_service_data": "{\"type\":\"WAFV2\",\"defaultAction\":{\"type\":\"ALLOW\"},...}"
  }
}
```

### 4. Feed policies into Terraform

`demo.sh` does this for you. From repo root:

```bash
chmod +x demo.sh

./demo.sh
```

What `demo.sh` does:

1. Ensures `generated/` exists.
2. Runs the Go renderer (unless `generated/policies.json` already exists).
3. Wraps `generated/policies.json` into `terraform/generated_policies.auto.tfvars.json`.
4. Runs `terraform apply` again to create `aws_fms_policy` resources using those policies.

After a successful run you should see:

* FMS policies in the **Firewall Manager console**.
* `terraform` outputs showing FMS policy ARNs and ALB DNS name.

### 5. Teardown (IMPORTANT)

When done:

```bash
cd terraform
terraform destroy
cd ..
```

This:

* Removes FMS policies.
* Tears down the ALB and networking resources.

---

## How It Works (High-level)

1. **Configuration-driven variants**
   `configs/policy-variants.yaml` defines how tags map to policy variants. e.g.:

   * ALBs with `env=prod` → add additional AWS-managed rule groups.
   * ALBs with `visibility=public` → attach a different set of rule groups.

2. **Discovery** (`internal/discovery`)
   The Go CLI uses AWS SDK v2 to:

   * `DescribeLoadBalancers`
   * `DescribeTags` per ALB
   * Emit a normalized `Resource` struct with type, ARN, and tag map.

3. **Policy building** (`internal/policy`)
   For each discovered resource:

   * Lookup `resourceDefaults` (e.g., for `alb`) from YAML.
   * Find matching variant(s) based on tag selectors.
   * Merge default + variant rule groups into a **policy model**.
   * Render `managed_service_data` JSON via `text/template` from `templates/fms_policy.tmpl`.
   * Validate the JSON with `encoding/json`.

4. **Terraform glue**

   * Go writes `generated/policies.json` as a map of **policy name → policy metadata**.
   * `demo.sh` wraps that into a Terraform `*.auto.tfvars.json` with `fms_policies_json` variable.
   * `terraform/main.tf` uses `for_each` to create `aws_fms_policy` per entry.


---

## Troubleshooting

Common issues:

1. **FMS not enabled / wrong account**

   * Error on `aws_fms_policy` creation about FMS policy permissions or Organizations.
   * Check that:

     * You are in the **delegated FMS admin account**.
     * FMS is enabled for the Org.
     * Your IAM principal has `fms:PutPolicy`.

2. **Region vs FMS vs WAFv2**

   * This demo uses **regional WAFv2** for **ALBs**.
   * Make sure your Go `-region` flag matches your ALB / Terraform region.

3. **AWS auth / environment**

   * If Go discovery fails with credential errors, verify `aws sts get-caller-identity`.
   * Check environment variables like `AWS_PROFILE`, `AWS_REGION`.

4. **Terraform JSON variable issues**

   * If `aws_fms_policy` errors with invalid JSON in `managed_service_data`, inspect:

     * `generated/policies.json`
     * `terraform/generated_policies.auto.tfvars.json`
   * Make sure the JSON is valid and not doubly encoded.

---

## General

* “Read `configs/policy-variants.yaml` and propose new variants for staging and dev.”
* “Refactor `internal/policy/policy.go` to support combining multiple variants per resource.”
* “Add logging to `internal/discovery/discovery.go` showing which tags are used to select variants.”

### CloudFront / API Gateway

* “Add support in `internal/discovery/discovery.go` to also discover API Gateway v2 HTTP APIs with tags, similar to ALBs.”
* “Extend `configs/policy-variants.yaml` and `internal/policy/policy.go` so we can generate FMS policies for CloudFront distributions using `resource_type = \"AWS::CloudFront::Distribution\"`.”
* “Update `templates/fms_policy.tmpl` to support different default actions (BLOCK vs ALLOW) based on tag-derived params.”

---

# Files

Below are all project files, ready to copy into a Git repo.

---

## `README.md`

> (You’re reading it now – save this content as `README.md`.)

---

## `AGENTS.md`

```markdown
# AGENTS: aws-fms-secpolicy-learning

## Mission

Demonstrate dynamic, tag-driven AWS Firewall Manager (FMS) + WAFv2 policies using:

- Go (AWS SDK v2, text/template, encoding/json)
- Terraform (aws_fms_policy)
- Simple config-driven policy variants

## Key Files

- `cmd/renderer/main.go` – CLI entrypoint.
- `internal/discovery/discovery.go` – Discovers WAF-attachable resources (ALBs).
- `internal/policy/policy.go` – Builds FMS/WAF policy models + renders `managed_service_data`.
- `configs/policy-variants.yaml` – Tag/resource-type → policy behavior.
- `templates/fms_policy.tmpl` – JSON template for WAFv2 `managed_service_data`.
- `terraform/main.tf` – Demo infra + FMS policies.
- `demo.sh` – Orchestrates discovery → render → terraform apply.

## Style Guidelines

- Use Go 1.21 conventions.
- Keep dependencies minimal (stdlib + `aws-sdk-go-v2` + `gopkg.in/yaml.v3`).
- Write small, focused functions.
- Favor composition over inheritance-like patterns.
- Add unit tests in `internal/...` and `tests/` when modifying behavior.

## Suggested Enhancements

1. **Add more resource types**
   - Discover and model:
     - CloudFront distributions
     - API Gateway HTTP APIs
     - Regional WAFv2 WebACLs
   - Adjust config & templates accordingly.

2. **Improve policy depth**
   - Use rate-based statements.
   - Use path- or header-based match conditions.
   - Hook in geo-based blocking rules.

3. **Org-wide scope**
   - Use include/exclude maps for OU/account scoping.
   - Introduce config to drive policy scoping per OU.

4. **DX improvements**
   - Add richer CLI flags (dry run, diff mode).
   - Emit human-readable table summaries of discovered resources and chosen variants.

When in doubt, prefer **small incremental changes** with comments that explain *why*, not just *what*.
```

---

## `go.mod`

```go
module github.com/your-org/aws-fms-secpolicy-learning

go 1.21

require (
	github.com/aws/aws-sdk-go-v2 v1.32.0
	github.com/aws/aws-sdk-go-v2/config v1.27.15
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.41.5
	gopkg.in/yaml.v3 v3.0.1
)
```

*(Versions are examples; run `go mod tidy` to resolve latest compatible versions.)*

---

## `cmd/renderer/main.go`

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/your-org/aws-fms-secpolicy-learning/internal/config"
	"github.com/your-org/aws-fms-secpolicy-learning/internal/discovery"
	"github.com/your-org/aws-fms-secpolicy-learning/internal/policy"
	"github.com/your-org/aws-fms-secpolicy-learning/internal/util"
)

var (
	flagDiscover = flag.Bool("discover", false, "Discover resources from AWS instead of reading -input JSON.")
	flagInput    = flag.String("input", "resources.json", "Input resources JSON file when -discover=false.")
	flagConfig   = flag.String("config", "configs/policy-variants.yaml", "Path to policy variants YAML config.")
	flagOutput   = flag.String("output", "generated/policies.json", "Path to write rendered policies JSON.")
	flagRegion   = flag.String("region", "", "AWS region for discovery (e.g. us-west-2). If empty, uses default config.")
)

func main() {
	flag.Parse()

	logger := util.NewLogger()

	if err := run(context.Background(), logger); err != nil {
		logger.Errorf("fatal error: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *util.Logger) error {
	logger.Infof("loading policy config from %s", *flagConfig)
	cfg, err := config.Load(*flagConfig)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var resources []discovery.Resource

	if *flagDiscover {
		logger.Infof("discovering resources from AWS")
		awsCfg, err := loadAWSConfig(ctx, *flagRegion)
		if err != nil {
			return fmt.Errorf("load AWS config: %w", err)
		}

		resources, err = discovery.DiscoverALBs(ctx, awsCfg, logger)
		if err != nil {
			return fmt.Errorf("discover ALBs: %w", err)
		}
		logger.Infof("discovered %d ALB resources", len(resources))
	} else {
		logger.Infof("reading resources from %s", *flagInput)
		data, err := os.ReadFile(*flagInput)
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}
		if err := json.Unmarshal(data, &resources); err != nil {
			return fmt.Errorf("unmarshal input resources: %w", err)
		}
		logger.Infof("loaded %d resources from file", len(resources))
	}

	if len(resources) == 0 {
		logger.Warnf("no resources discovered or loaded; nothing to do")
		return nil
	}

	logger.Infof("building policies from resources")
	rendered, err := policy.BuildPolicies(resources, cfg, logger)
	if err != nil {
		return fmt.Errorf("build policies: %w", err)
	}

	if err := os.MkdirAll("generated", 0o755); err != nil {
		return fmt.Errorf("ensure generated dir: %w", err)
	}

	outputBytes, err := json.MarshalIndent(rendered, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal policies to JSON: %w", err)
	}

	if err := os.WriteFile(*flagOutput, outputBytes, 0o644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}

	logger.Infof("wrote %d policies to %s", len(rendered), *flagOutput)
	return nil
}

func loadAWSConfig(ctx context.Context, region string) (awsCfg aws.Config, err error) {
	if region != "" {
		awsCfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region))
	} else {
		awsCfg, err = config.LoadDefaultConfig(ctx)
	}
	return
}
```

---

## `internal/util/logging.go`

```go
package util

import (
	"log"
	"os"
)

type Logger struct {
	l *log.Logger
}

func NewLogger() *Logger {
	return &Logger{
		l: log.New(os.Stderr, "[fms-poc] ", log.LstdFlags|log.Lshortfile),
	}
}

func (l *Logger) Infof(format string, args ...any) {
	l.l.Printf("INFO: "+format, args...)
}

func (l *Logger) Warnf(format string, args ...any) {
	l.l.Printf("WARN: "+format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.l.Printf("ERROR: "+format, args...)
}
```

---

## `internal/config/config.go`

```go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PolicyConfig is the root config structure loaded from policy-variants.yaml.
type PolicyConfig struct {
	// ResourceDefaults define baseline behavior per resource type.
	// Key is a logical type like "alb", "apigw", "cloudfront".
	ResourceDefaults map[string]ResourceDefaults `yaml:"resourceDefaults"`

	// Variants contain tag-based overrides to apply on top of defaults.
	Variants []Variant `yaml:"variants"`
}

// ResourceDefaults describe default WAF/FMS settings for a resource type.
type ResourceDefaults struct {
	// ResourceType is the FMS resource type string, e.g.
	// "AWS::ElasticLoadBalancingV2::LoadBalancer"
	ResourceType string `yaml:"resourceType"`

	// Scope controls WAFv2 scope (REGIONAL vs CLOUDFRONT).
	Scope string `yaml:"scope"`

	// DefaultAction is typically "ALLOW" or "BLOCK".
	DefaultAction string `yaml:"defaultAction"`

	// ManagedRuleGroups always applied for this resource type.
	ManagedRuleGroups []ManagedRuleGroupConfig `yaml:"managedRuleGroups"`
}

// Variant defines tag matches and overrides.
type Variant struct {
	// Name is a friendly name for this variant.
	Name string `yaml:"name"`

	// Match is a tag selector; all key/value pairs must match.
	Match map[string]string `yaml:"match"`

	// Overrides define extra behavior layered onto defaults.
	Overrides VariantOverrides `yaml:"overrides"`
}

// VariantOverrides describe differences from the defaults.
// This is intentionally small and can grow over time.
type VariantOverrides struct {
	// ExtraManagedRuleGroups adds more AWS-managed rule groups.
	ExtraManagedRuleGroups []ManagedRuleGroupConfig `yaml:"extraManagedRuleGroups"`

	// OverrideDefaultAction (if non-empty) replaces the default action.
	OverrideDefaultAction string `yaml:"overrideDefaultAction"`
}

// ManagedRuleGroupConfig identifies an AWS-managed rule group.
type ManagedRuleGroupConfig struct {
	Vendor string `yaml:"vendor"`
	Name   string `yaml:"name"`
}

// Load loads PolicyConfig from a YAML file.
func Load(path string) (*PolicyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg PolicyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal YAML: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate performs basic validation of the config.
func (c *PolicyConfig) Validate() error {
	if len(c.ResourceDefaults) == 0 {
		return fmt.Errorf("resourceDefaults must not be empty")
	}
	for key, rd := range c.ResourceDefaults {
		if rd.ResourceType == "" {
			return fmt.Errorf("resourceDefaults[%s].resourceType is required", key)
		}
		if rd.Scope == "" {
			return fmt.Errorf("resourceDefaults[%s].scope is required", key)
		}
		if rd.DefaultAction == "" {
			return fmt.Errorf("resourceDefaults[%s].defaultAction is required", key)
		}
	}
	return nil
}
```

---

## `internal/discovery/discovery.go`

```go
package discovery

import (
	"context"
	"fmt"
	"strings"

	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/your-org/aws-fms-secpolicy-learning/internal/util"
)

// ResourceType is a logical type understood by this PoC.
type ResourceType string

const (
	ResourceTypeALB ResourceType = "alb"
	// In the future, extend with:
	// ResourceTypeAPIGateway ResourceType = "apigw"
	// ResourceTypeCloudFront ResourceType = "cloudfront"
)

// Resource is a simplified, taggable representation of a WAF-attachable resource.
type Resource struct {
	ID   string            `json:"id"`
	ARN  string            `json:"arn"`
	Type ResourceType      `json:"type"`
	Tags map[string]string `json:"tags"`
}

// DiscoverALBs discovers Application Load Balancers and their tags.
//
// This PoC focuses on ALBs. You can extend this function (or add new ones)
// to discover API Gateway or CloudFront resources.
//
func DiscoverALBs(ctx context.Context, cfg aws.Config, logger *util.Logger) ([]Resource, error) {
	client := elbv2.NewFromConfig(cfg)

	var (
		resources []Resource
		marker    *string
	)

	for {
		out, err := client.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
			Marker: marker,
		})
		if err != nil {
			return nil, fmt.Errorf("describe load balancers: %w", err)
		}
		if len(out.LoadBalancers) == 0 && marker == nil {
			logger.Warnf("no load balancers found in this region")
		}

		albArns := make([]string, 0, len(out.LoadBalancers))
		for _, lb := range out.LoadBalancers {
			// We only care about application load balancers.
			if lb.Type != types.LoadBalancerTypeEnumApplication {
				continue
			}
			if lb.LoadBalancerArn == nil {
				continue
			}
			albArns = append(albArns, *lb.LoadBalancerArn)
		}

		if len(albArns) > 0 {
			tagOut, err := client.DescribeTags(ctx, &elbv2.DescribeTagsInput{
				ResourceArns: albArns,
			})
			if err != nil {
				return nil, fmt.Errorf("describe tags: %w", err)
			}

			for _, td := range tagOut.TagDescriptions {
				if td.ResourceArn == nil {
					continue
				}
				arn := *td.ResourceArn
				id := lbNameFromArn(arn)
				tags := make(map[string]string, len(td.Tags))
				for _, t := range td.Tags {
					if t.Key == nil || t.Value == nil {
						continue
					}
					tags[*t.Key] = *t.Value
				}
				resources = append(resources, Resource{
					ID:   id,
					ARN:  arn,
					Type: ResourceTypeALB,
					Tags: tags,
				})
			}
		}

		if out.NextMarker == nil || *out.NextMarker == "" {
			break
		}
		marker = out.NextMarker
	}

	return resources, nil
}

// lbNameFromArn extracts a human-readable LB identifier from an ARN.
// Example ARN:
//   arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/demo-alb/abcd1234
func lbNameFromArn(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) == 0 {
		return arn
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}
```

---

## `internal/policy/policy.go`

```go
package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/your-org/aws-fms-secpolicy-learning/internal/config"
	"github.com/your-org/aws-fms-secpolicy-learning/internal/discovery"
	"github.com/your-org/aws-fms-secpolicy-learning/internal/util"
)

// RenderedPolicy is the JSON-friendly structure written to generated/policies.json.
//
// Terraform reads this structure and turns each entry into an aws_fms_policy.
type RenderedPolicy struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	ResourceType       string `json:"resource_type"`
	Scope              string `json:"scope"`
	ManagedServiceData string `json:"managed_service_data"`
}

// BuildPolicies generates FMS policies from discovered resources and config.
func BuildPolicies(resources []discovery.Resource, cfg *config.PolicyConfig, logger *util.Logger) (map[string]RenderedPolicy, error) {
	tmpl, err := template.New("fms_policy").
		Funcs(template.FuncMap{
			// joinStrings is a minimal helper; can add more as needed.
			"joinStrings": func(sep string, elems []string) string {
				var buf bytes.Buffer
				for i, e := range elems {
					if i > 0 {
						buf.WriteString(sep)
					}
					buf.WriteString(e)
				}
				return buf.String()
			},
		}).
		ParseFS(templateFS, "templates/fms_policy.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	result := make(map[string]RenderedPolicy)

	for _, r := range resources {
		switch r.Type {
		case discovery.ResourceTypeALB:
			if err := buildForALB(r, cfg, tmpl, result, logger); err != nil {
				return nil, err
			}
		default:
			logger.Warnf("unknown resource type %q, skipping", r.Type)
		}
	}

	return result, nil
}

func buildForALB(
	res discovery.Resource,
	cfg *config.PolicyConfig,
	tmpl *template.Template,
	out map[string]RenderedPolicy,
	logger *util.Logger,
) error {
	defaults, ok := cfg.ResourceDefaults["alb"]
	if !ok {
		logger.Warnf("no resourceDefaults for 'alb'; skipping resource %s", res.ARN)
		return nil
	}

	var matchedVariants []config.Variant
	for _, v := range cfg.Variants {
		if matchesTags(res.Tags, v.Match) {
			matchedVariants = append(matchedVariants, v)
		}
	}

	if len(matchedVariants) == 0 {
		logger.Infof("resource %s has no matching variants; using defaults only", res.ARN)
	}

	var chosenVariant *config.Variant
	if len(matchedVariants) > 0 {
		chosenVariant = &matchedVariants[0]
		logger.Infof("resource %s matched variant %s", res.ARN, chosenVariant.Name)
	}

	model := buildTemplateModel(defaults, chosenVariant)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, model); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	// Validate JSON structure.
	if !json.Valid(buf.Bytes()) {
		return fmt.Errorf("rendered managed_service_data is not valid JSON for resource %s", res.ARN)
	}

	policyName := fmt.Sprintf("auto-alb-%s", sanitizeName(res.ID))
	desc := "Auto-generated WAFv2 policy for ALBs"

	p := RenderedPolicy{
		Name:               policyName,
		Description:        desc,
		ResourceType:       defaults.ResourceType,
		Scope:              defaults.Scope,
		ManagedServiceData: buf.String(), // JSON string
	}
	out[policyName] = p
	return nil
}

func matchesTags(resourceTags map[string]string, match map[string]string) bool {
	if len(match) == 0 {
		return false
	}
	for k, v := range match {
		if resourceTags[k] != v {
			return false
		}
	}
	return true
}

func sanitizeName(s string) string {
	// Very small sanitizer to keep policy names TF/HCL-friendly.
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case 'a' <= r && r <= 'z',
			'A' <= r && r <= 'Z',
			'0' <= r && r <= '9',
			r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}

// templateFS is populated via go:embed to read templates at runtime.
//
//go:generate go env -w GO111MODULE=on
//go:embed ../../templates/*
var templateFS embed.FS

// TemplateModel is fed into fms_policy.tmpl.
type TemplateModel struct {
	Type            string                       `json:"type"`
	DefaultAction   string                       `json:"defaultAction"`
	Scope           string                       `json:"scope"`
	RuleGroups      []TemplateRuleGroup          `json:"ruleGroups"`
	OverrideDefault string                       `json:"overrideDefault,omitempty"`
}

type TemplateRuleGroup struct {
	Vendor string `json:"vendor"`
	Name   string `json:"name"`
}

func buildTemplateModel(
	defaults config.ResourceDefaults,
	variant *config.Variant,
) TemplateModel {
	ruleGroups := make([]TemplateRuleGroup, 0, len(defaults.ManagedRuleGroups))
	for _, rg := range defaults.ManagedRuleGroups {
		ruleGroups = append(ruleGroups, TemplateRuleGroup{
			Vendor: rg.Vendor,
			Name:   rg.Name,
		})
	}

	defaultAction := defaults.DefaultAction
	if variant != nil && variant.Overrides.OverrideDefaultAction != "" {
		defaultAction = variant.Overrides.OverrideDefaultAction
	}

	if variant != nil {
		for _, rg := range variant.Overrides.ExtraManagedRuleGroups {
			ruleGroups = append(ruleGroups, TemplateRuleGroup{
				Vendor: rg.Vendor,
				Name:   rg.Name,
			})
		}
	}

	return TemplateModel{
		Type:          "WAFV2",
		DefaultAction: defaultAction,
		Scope:         defaults.Scope,
		RuleGroups:    ruleGroups,
	}
}
```

> **Note:** This file uses `go:embed`. When you create the repo, ensure `go` 1.16+ is used (we target 1.21).

---

## `configs/policy-variants.yaml`

```yaml
# Config-driven mapping from resource types + tags to FMS/WAF behavior.

resourceDefaults:
  alb:
    resourceType: "AWS::ElasticLoadBalancingV2::LoadBalancer"
    scope: "REGIONAL"
    defaultAction: "ALLOW"
    managedRuleGroups:
      # Base rule groups for all ALBs in the demo.
      - vendor: "AWS"
        name: "AWSManagedRulesCommonRuleSet"

variants:
  # 1) Strict rules for prod ALBs.
  - name: "prod-strict"
    match:
      env: "prod"
    overrides:
      overrideDefaultAction: "ALLOW"
      extraManagedRuleGroups:
        - vendor: "AWS"
          name: "AWSManagedRulesKnownBadInputsRuleSet"
        - vendor: "AWS"
          name: "AWSManagedRulesAmazonIpReputationList"

  # 2) Public-facing ALBs get bot control rules.
  - name: "public-bot-protection"
    match:
      visibility: "public"
    overrides:
      extraManagedRuleGroups:
        - vendor: "AWS"
          name: "AWSManagedRulesBotControlRuleSet"

  # 3) High-sensitivity tags add anonymity / bad input protections.
  - name: "high-sensitivity-hardened"
    match:
      sensitivity: "Dhigh"
    overrides:
      extraManagedRuleGroups:
        - vendor: "AWS"
          name: "AWSManagedRulesAnonymousIpList"
        - vendor: "AWS"
          name: "AWSManagedRulesSQLiRuleSet"
```

---

## `templates/fms_policy.tmpl`

This template renders the `managed_service_data` JSON expected by FMS for WAFv2.

```json
{
  "type": "WAFV2",
  "defaultAction": {
    "type": "{{ .DefaultAction }}"
  },
  "overrideCustomerWebACLAssociation": false,
  "postProcessRuleGroups": [],
  "preProcessRuleGroups": [
    {{- range $index, $rg := .RuleGroups -}}
    {{- if gt $index 0 }},{{ end }}
    {
      "ruleGroupType": "ManagedRuleGroup",
      "managedRuleGroupIdentifier": {
        "vendorName": "{{ $rg.Vendor }}",
        "managedRuleGroupName": "{{ $rg.Name }}"
      },
      "overrideAction": {
        "type": "NONE"
      },
      "excludeRules": []
    }
    {{- end }}
  ]
}
```

---

## `resources.json` (sample input for non-discovery mode)

```json
[
  {
    "id": "demo-alb/abcd1234",
    "arn": "arn:aws:elasticloadbalancing:us-west-2:111122223333:loadbalancer/app/demo-alb/abcd1234",
    "type": "alb",
    "tags": {
      "env": "prod",
      "visibility": "public",
      "sensitivity": "Dhigh"
    }
  }
]
```

---

## Terraform

### `terraform/provider.tf`

```hcl
terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}
```

---

### `terraform/variables.tf`

```hcl
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
```

---

### `terraform/main.tf`

```hcl
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
```

---

### `terraform/outputs.tf`

```hcl
output "alb_dns_name" {
  description = "DNS name of the demo ALB"
  value       = aws_lb.demo.dns_name
}

output "fms_policy_arns" {
  description = "ARNs of created FMS policies"
  value       = [for p in aws_fms_policy.waf_v2 : p.arn]
}
```

---

### `terraform/modules/fms-setup/main.tf`

```hcl
variable "create_placeholder" {
  type    = bool
  default = true
}

# This module is intentionally minimal; it represents "Org-level FMS setup"
# which typically happens once and is not managed by this PoC.
#
# - Data lookups for current FMS admin
# - Validation that Organizations + FMS are enabled

resource "null_resource" "placeholder" {
  count = var.create_placeholder ? 1 : 0

  provisioner "local-exec" {
    command = "echo 'FMS admin setup is assumed to be done outside of this PoC.'"
  }
}
```

---

### `terraform/modules/fms-setup/variables.tf`

```hcl
variable "create_placeholder" {
  type    = bool
  default = true
}
```

---

### `terraform/modules/fms-setup/outputs.tf`

```hcl
output "placeholder_message" {
  value = "FMS admin setup is assumed external to this PoC."
}
```

---

## `demo.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GEN_DIR="${ROOT_DIR}/generated"
POLICIES_JSON="${GEN_DIR}/policies.json"
TF_DIR="${ROOT_DIR}/terraform"
TF_VARS_JSON="${TF_DIR}/generated_policies.auto.tfvars.json"

echo "[demo] ensuring generated directory exists"
mkdir -p "${GEN_DIR}"

if [ ! -f "${POLICIES_JSON}" ]; then
  echo "[demo] policies.json not found; running Go renderer with discovery"

  go run "${ROOT_DIR}/cmd/renderer" \
    -discover \
    -region "us-west-2" \
    -config "${ROOT_DIR}/configs/policy-variants.yaml" \
    -output "${POLICIES_JSON}"
else
  echo "[demo] found existing ${POLICIES_JSON}; skipping discovery/render"
fi

echo "[demo] wrapping policies.json into Terraform tfvars"

cat > "${TF_VARS_JSON}" <<EOF
{
  "fms_policies_json": $(cat "${POLICIES_JSON}")
}
EOF

echo "[demo] applying Terraform configuration with rendered FMS policies"

cd "${TF_DIR}"
terraform init -input=false
terraform apply -auto-approve

echo "[demo] done. Check the outputs above and the AWS Console (Firewall Manager + ALB)."
```

---

# That’s it 🎉

You now have a complete, copy-pasteable PoC repo:

* Go-based **dynamic FMS/WAFv2 policy renderer**
* Terraform-based **demo infra + policy application**
* Config-driven, extensible structure with clear extension hooks

Next steps you might try:

* Add **CloudFront** or **API Gateway** discovery.
* Model **OU/account scoping** via FMS include/exclude maps.
* Enforce **different policy sets** for `dev`, `stage`, and `prod`.

Happy hacking — and feel free to keep evolving this repo.

