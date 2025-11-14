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

	// Match resources against the configured variants so that tagged assets
	// can receive tailored rule groups or default actions.
	var matchedVariants []config.Variant
	for _, v := range cfg.Variants {
		if matchesTags(res.Tags, v.Match) {
			matchedVariants = append(matchedVariants, v)
		}
	}

	if len(matchedVariants) == 0 {
		logger.Infof("resource %s has no matching variants; using defaults only", res.ARN)
	}

	// AI EXTENSION: Allow multiple variants to merge; for now we only use the first.
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

// TemplateModel is fed into fms_policy.tmpl.
type TemplateModel struct {
	Type            string              `json:"type"`
	DefaultAction   string              `json:"defaultAction"`
	Scope           string              `json:"scope"`
	RuleGroups      []TemplateRuleGroup `json:"ruleGroups"`
	OverrideDefault string              `json:"overrideDefault,omitempty"`
}

type TemplateRuleGroup struct {
	Vendor string `json:"vendor"`
	Name   string `json:"name"`
}

func buildTemplateModel(
	defaults config.ResourceDefaults,
	variant *config.Variant,
) TemplateModel {
	// Begin with the base rule groups applied to every resource of this type.
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
		// Layer on variant extras after defaults so the rendered order mirrors the config.
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
