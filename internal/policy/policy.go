package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/config"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/discovery"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/util"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/templates"
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
	// Parse the template once so every resource reuses the same compiled structure.
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
		ParseFS(templates.FS, "fms_policy.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	tmpl = tmpl.Lookup("fms_policy.tmpl")
	if tmpl == nil {
		return nil, fmt.Errorf("template fms_policy.tmpl not found after parsing")
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

	primaryValue := selectRuleSetValue(res.Tags, cfg.TagKeys.Primary, cfg.Defaults.Primary, cfg.RuleSets.Primary, logger, res.ARN)
	secondaryValue := selectRuleSetValue(res.Tags, cfg.TagKeys.Secondary, cfg.Defaults.Secondary, cfg.RuleSets.Secondary, logger, res.ARN)

	model := buildTemplateModel(defaults, cfg.RuleSets.Primary[primaryValue], cfg.RuleSets.Secondary[secondaryValue])

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, model); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	// Validate JSON structure.
	if !json.Valid(buf.Bytes()) {
		return fmt.Errorf("rendered managed_service_data is not valid JSON for resource %s", res.ARN)
	}

	policyName := fmt.Sprintf("auto-alb-%s", sanitizeName(res.ID))
	desc := fmt.Sprintf("Auto-generated WAFv2 policy (primary=%s, secondary=%s)", primaryValue, secondaryValue)

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

func selectRuleSetValue(
	tags map[string]string,
	tagKey string,
	defaultValue string,
	available map[string]config.RuleSet,
	logger *util.Logger,
	resourceARN string,
) string {
	value := tags[tagKey]
	if value == "" {
		return defaultValue
	}
	if _, ok := available[value]; ok {
		return value
	}
	logger.Warnf("resource %s has tag %s=%s which is not configured; using default %s", resourceARN, tagKey, value, defaultValue)
	return defaultValue
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

// TemplateModel is fed into fms_policy.tmpl.
type TemplateModel struct {
	Type            string              `json:"type"`
	DefaultAction   string              `json:"defaultAction"`
	Scope           string              `json:"scope"`
	RuleGroups      []TemplateRuleGroup `json:"ruleGroups"`
	OverrideDefault string              `json:"overrideDefault,omitempty"`
}

type TemplateRuleGroup struct {
	Type   string `json:"type"`
	Vendor string `json:"vendor,omitempty"`
	Name   string `json:"name,omitempty"`
	ARN    string `json:"arn,omitempty"`
}

func buildTemplateModel(
	defaults config.ResourceDefaults,
	primary config.RuleSet,
	secondary config.RuleSet,
) TemplateModel {
	// Begin with the base rule groups applied to every resource of this type.
	merged := mergeRuleGroups(
		defaults.ManagedRuleGroups,
		primary.RuleGroups,
		secondary.RuleGroups,
	)

	ruleGroups := make([]TemplateRuleGroup, 0, len(merged))
	for _, rg := range merged {
		t := TemplateRuleGroup{}
		if rg.ARN != "" {
			t.Type = "RuleGroup"
			t.ARN = rg.ARN
		} else {
			t.Type = "ManagedRuleGroup"
			t.Vendor = rg.Vendor
			t.Name = rg.Name
		}
		ruleGroups = append(ruleGroups, t)
	}

	return TemplateModel{
		Type:          "WAFV2",
		DefaultAction: defaults.DefaultAction,
		Scope:         defaults.Scope,
		RuleGroups:    ruleGroups,
	}
}

func mergeRuleGroups(groups ...[]config.RuleGroupConfig) []config.RuleGroupConfig {
	seen := make(map[string]bool)
	out := make([]config.RuleGroupConfig, 0)

	for _, list := range groups {
		for _, rg := range list {
			key := rg.ARN
			if key == "" {
				key = rg.Vendor + "/" + rg.Name
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, rg)
		}
	}

	return out
}
