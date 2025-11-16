package policy

import (
	"encoding/json"
	"testing"

	"github.com/forkedpacket/aws-fms-secpolicy-learning/configs"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/config"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/discovery"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/util"
)

type renderedServiceData struct {
	PreProcessRuleGroups []struct {
		RuleGroupArn               string `json:"ruleGroupArn"`
		ManagedRuleGroupIdentifier struct {
			VendorName           string `json:"vendorName"`
			ManagedRuleGroupName string `json:"managedRuleGroupName"`
		} `json:"managedRuleGroupIdentifier"`
	} `json:"preProcessRuleGroups"`
}

func TestBuildPolicies_PrimarySecondarySelection(t *testing.T) {
	cfg := mustLoadConfig(t)

	resources := []discovery.Resource{
		{
			ID:   "demo-alb/abcd",
			ARN:  "arn:aws:elasticloadbalancing:::loadbalancer/app/demo-alb/abcd",
			Type: discovery.ResourceTypeALB,
			Tags: map[string]string{
				cfg.TagKeys.Primary:   "ou-shared-edge",
				cfg.TagKeys.Secondary: "ou-shared-bot",
			},
		},
	}

	logger := util.NewLogger()
	result, err := BuildPolicies(resources, cfg, logger)
	if err != nil {
		t.Fatalf("build policies: %v", err)
	}
	p, ok := result["auto-alb-demo-alb-abcd"]
	if !ok {
		t.Fatalf("policy not found in result")
	}

	var msd renderedServiceData
	if err := json.Unmarshal([]byte(p.ManagedServiceData), &msd); err != nil {
		t.Fatalf("unmarshal managed_service_data: %v", err)
	}

	wantGroups := map[string]bool{
		"AWS/AWSManagedRulesCommonRuleSet": false, // baseline managed
		"arn:aws:wafv2:us-west-2:123456789012:regional/rulegroup/ou-shared-edge/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee": false,
		"arn:aws:wafv2:us-west-2:123456789012:regional/rulegroup/ou-shared-bot/cccccccc-dddd-eeee-ffff-111111111111":  false,
	}

	for _, rg := range msd.PreProcessRuleGroups {
		key := rg.RuleGroupArn
		if key == "" {
			key = rg.ManagedRuleGroupIdentifier.VendorName + "/" + rg.ManagedRuleGroupIdentifier.ManagedRuleGroupName
		}
		if _, exists := wantGroups[key]; exists {
			wantGroups[key] = true
		}
	}

	for key, seen := range wantGroups {
		if !seen {
			t.Fatalf("expected rule group %s to be present", key)
		}
	}
}

func TestBuildPolicies_DefaultsUsedWhenTagsMissing(t *testing.T) {
	cfg := mustLoadConfig(t)

	resources := []discovery.Resource{
		{
			ID:   "demo-alb/efgh",
			ARN:  "arn:aws:elasticloadbalancing:::loadbalancer/app/demo-alb/efgh",
			Type: discovery.ResourceTypeALB,
			Tags: map[string]string{},
		},
	}

	logger := util.NewLogger()
	result, err := BuildPolicies(resources, cfg, logger)
	if err != nil {
		t.Fatalf("build policies: %v", err)
	}
	p, ok := result["auto-alb-demo-alb-efgh"]
	if !ok {
		t.Fatalf("policy not found in result")
	}

	var msd renderedServiceData
	if err := json.Unmarshal([]byte(p.ManagedServiceData), &msd); err != nil {
		t.Fatalf("unmarshal managed_service_data: %v", err)
	}

	seen := map[string]bool{}
	for _, rg := range msd.PreProcessRuleGroups {
		key := rg.RuleGroupArn
		if key == "" {
			key = rg.ManagedRuleGroupIdentifier.VendorName + "/" + rg.ManagedRuleGroupIdentifier.ManagedRuleGroupName
		}
		seen[key] = true
	}

	if !seen["AWS/AWSManagedRulesCommonRuleSet"] {
		t.Fatalf("expected default baseline rule group")
	}
	if !seen["arn:aws:wafv2:us-west-2:123456789012:regional/rulegroup/ou-shared-bot/cccccccc-dddd-eeee-ffff-111111111111"] {
		t.Fatalf("expected default secondary rule group from defaults.secondary")
	}
}

func mustLoadConfig(t *testing.T) *config.PolicyConfig {
	t.Helper()
	cfg, err := config.LoadFromBytes(configs.EmbeddedPolicyVariants)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}
