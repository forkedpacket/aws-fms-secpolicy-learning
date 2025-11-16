package fmsapply

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/fms"
	fmstypes "github.com/aws/aws-sdk-go-v2/service/fms/types"

	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/policy"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/util"
)

// UpsertPolicy ensures the FMS policy exists (create or update) for the provided managed_service_data payload.
// If dryRun is true, it only logs the intended change.
func UpsertPolicy(ctx context.Context, client *fms.Client, p policy.RenderedPolicy, ouID string, dryRun bool, logger *util.Logger) error {
	includeMap := map[string][]string{}
	if ouID != "" {
		includeMap["ORG_UNIT"] = []string{ouID}
	}

	policyInput := fmstypes.Policy{
		ExcludeResourceTags: false,
		RemediationEnabled:  true,
		ResourceTypeList:    []string{p.ResourceType},
		PolicyName:          aws.String(p.Name),
		PolicyDescription:   aws.String(p.Description),
		SecurityServicePolicyData: &fmstypes.SecurityServicePolicyData{
			Type:               fmstypes.SecurityServiceTypeWafv2,
			ManagedServiceData: aws.String(p.ManagedServiceData),
		},
		IncludeMap: includeMap,
	}

	existingID, updateToken, err := findPolicyByName(ctx, client, p.Name)
	if err != nil {
		return fmt.Errorf("find existing policy %s: %w", p.Name, err)
	}
	if existingID != nil {
		if updateToken == nil {
			return fmt.Errorf("existing policy %s missing update token", p.Name)
		}
		policyInput.PolicyId = existingID
		policyInput.PolicyUpdateToken = updateToken
		logger.Infof("updating existing policy %s", p.Name)
	} else {
		logger.Infof("creating new policy %s", p.Name)
	}

	if dryRun {
		logger.Infof("dry-run enabled; skipping PutPolicy for %s", p.Name)
		return nil
	}

	if _, err := client.PutPolicy(ctx, &fms.PutPolicyInput{Policy: &policyInput}); err != nil {
		return fmt.Errorf("put policy %s: %w", p.Name, err)
	}
	return nil
}

func findPolicyByName(ctx context.Context, client *fms.Client, name string) (*string, *string, error) {
	pager := fms.NewListPoliciesPaginator(client, &fms.ListPoliciesInput{})

	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list policies: %w", err)
		}
		for _, summary := range page.PolicyList {
			if summary.PolicyName != nil && *summary.PolicyName == name {
				if summary.PolicyId == nil {
					return nil, nil, fmt.Errorf("policy %s found without id", name)
				}

				policyID := summary.PolicyId
				gp, err := client.GetPolicy(ctx, &fms.GetPolicyInput{PolicyId: policyID})
				if err != nil {
					return nil, nil, fmt.Errorf("get policy %s: %w", name, err)
				}

				var updateToken *string
				if gp.Policy != nil {
					updateToken = gp.Policy.PolicyUpdateToken
				}
				return policyID, updateToken, nil
			}
		}
	}

	return nil, nil, nil
}
