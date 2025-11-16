package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PolicyConfig is the root config structure loaded from policy-variants.yaml.
// It maps two tag keys (primary/secondary) to rule groups applied to each resource.
type PolicyConfig struct {
	// ResourceDefaults define baseline behavior per resource type.
	// Key is a logical type like "alb", "apigw", "cloudfront".
	ResourceDefaults map[string]ResourceDefaults `yaml:"resourceDefaults"`

	// TagKeys configure which tag names the Lambda/CLI reads from resources.
	TagKeys TagKeys `yaml:"tagKeys"`

	// RuleSets maps tag values to rule groups for each tag key.
	RuleSets RuleSets `yaml:"ruleSets"`

	// Defaults define which rule set names to fall back to when a tag is missing.
	Defaults RuleSetDefaults `yaml:"defaults"`
}

// TagKeys controls the tag names used for rule selection.
type TagKeys struct {
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary"`
}

// RuleSets holds named rule sets per tag key.
type RuleSets struct {
	Primary   map[string]RuleSet `yaml:"primary"`
	Secondary map[string]RuleSet `yaml:"secondary"`
}

// RuleSetDefaults specify which rule set names to use when tags are absent or invalid.
type RuleSetDefaults struct {
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary"`
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
	ManagedRuleGroups []RuleGroupConfig `yaml:"managedRuleGroups"`
}

// RuleSet represents a named collection of rule groups chosen by tag value.
type RuleSet struct {
	RuleGroups []RuleGroupConfig `yaml:"ruleGroups"`
}

// RuleGroupConfig identifies either an AWS-managed rule group (vendor/name) or a customer-managed rule group (arn).
// Exactly one of (ARN) or (Vendor + Name) must be set.
type RuleGroupConfig struct {
	ARN    string `yaml:"arn"`
	Vendor string `yaml:"vendor"`
	Name   string `yaml:"name"`
}

// Load loads PolicyConfig from a YAML file.
func Load(path string) (*PolicyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes loads PolicyConfig from YAML bytes; useful for embedded defaults.
func LoadFromBytes(data []byte) (*PolicyConfig, error) {
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
		for i, rg := range rd.ManagedRuleGroups {
			if err := validateRuleGroup(fmt.Sprintf("resourceDefaults[%s].managedRuleGroups[%d]", key, i), rg); err != nil {
				return err
			}
		}
	}

	if c.TagKeys.Primary == "" || c.TagKeys.Secondary == "" {
		return fmt.Errorf("tagKeys.primary and tagKeys.secondary are required")
	}

	if c.RuleSets.Primary == nil || c.RuleSets.Secondary == nil {
		return fmt.Errorf("ruleSets.primary and ruleSets.secondary must be provided")
	}

	if c.Defaults.Primary == "" {
		return fmt.Errorf("defaults.primary is required")
	}
	if _, ok := c.RuleSets.Primary[c.Defaults.Primary]; !ok {
		return fmt.Errorf("defaults.primary %q not found in ruleSets.primary", c.Defaults.Primary)
	}

	if c.Defaults.Secondary == "" {
		return fmt.Errorf("defaults.secondary is required")
	}
	if _, ok := c.RuleSets.Secondary[c.Defaults.Secondary]; !ok {
		return fmt.Errorf("defaults.secondary %q not found in ruleSets.secondary", c.Defaults.Secondary)
	}

	for name, rs := range c.RuleSets.Primary {
		for i, rg := range rs.RuleGroups {
			if err := validateRuleGroup(fmt.Sprintf("ruleSets.primary[%s].ruleGroups[%d]", name, i), rg); err != nil {
				return err
			}
		}
	}
	for name, rs := range c.RuleSets.Secondary {
		for i, rg := range rs.RuleGroups {
			if err := validateRuleGroup(fmt.Sprintf("ruleSets.secondary[%s].ruleGroups[%d]", name, i), rg); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateRuleGroup(prefix string, rg RuleGroupConfig) error {
	hasARN := rg.ARN != ""
	hasManaged := rg.Vendor != "" || rg.Name != ""

	switch {
	case hasARN && hasManaged:
		return fmt.Errorf("%s: specify either arn OR vendor/name, not both", prefix)
	case hasARN:
		return nil
	case rg.Vendor != "" && rg.Name != "":
		return nil
	default:
		return fmt.Errorf("%s: must provide arn or vendor/name", prefix)
	}
}
