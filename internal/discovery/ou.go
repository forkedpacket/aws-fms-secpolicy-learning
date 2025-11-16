package discovery

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/util"
)

// AccountInOU verifies whether the current account belongs to the provided OU.
// If ouID is empty, it returns true.
func AccountInOU(ctx context.Context, cfg aws.Config, ouID string, logger *util.Logger) (bool, error) {
	if ouID == "" {
		return true, nil
	}

	currentAccount, err := currentAccountID(ctx, cfg)
	if err != nil {
		return false, fmt.Errorf("get current account id: %w", err)
	}

	orgClient := organizations.NewFromConfig(cfg)
	p := organizations.NewListAccountsForParentPaginator(orgClient, &organizations.ListAccountsForParentInput{
		ParentId: aws.String(ouID),
	})

	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return false, fmt.Errorf("list accounts for OU %s: %w", ouID, err)
		}
		for _, acct := range page.Accounts {
			if acct.Id != nil && *acct.Id == currentAccount {
				return true, nil
			}
		}
	}

	logger.Warnf("current account %s not found in OU %s", currentAccount, ouID)
	return false, nil
}

func currentAccountID(ctx context.Context, cfg aws.Config) (string, error) {
	stsClient := sts.NewFromConfig(cfg)
	out, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	if out.Account == nil {
		return "", fmt.Errorf("caller identity missing account")
	}
	return *out.Account, nil
}
