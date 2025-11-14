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
