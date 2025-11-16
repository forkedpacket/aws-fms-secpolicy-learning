package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/fms"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	"github.com/forkedpacket/aws-fms-secpolicy-learning/configs"
	policyconfig "github.com/forkedpacket/aws-fms-secpolicy-learning/internal/config"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/discovery"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/fmsapply"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/policy"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/util"
)

type Event struct {
	// DryRun skips calling PutPolicy and only logs intended actions.
	DryRun bool `json:"dryRun"`
	// Region allows overriding AWS region (optional).
	Region string `json:"region"`
}

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context, event Event) (string, error) {
	logger := util.NewLogger()

	awsCfg, err := loadAWSConfig(ctx, event.Region)
	if err != nil {
		logger.Errorf("load AWS config: %v", err)
		return "", err
	}

	cfg, err := loadPolicyConfig(ctx, awsCfg, logger)
	if err != nil {
		logger.Errorf("load policy config: %v", err)
		return "", err
	}

	ouID := os.Getenv("OU_ID")
	inOU, err := discovery.AccountInOU(ctx, awsCfg, ouID, logger)
	if err != nil {
		return "", fmt.Errorf("verify OU membership: %w", err)
	}
	if !inOU {
		return "account not in target OU; skipping", nil
	}

	resources, err := discovery.DiscoverALBs(ctx, awsCfg, logger)
	if err != nil {
		return "", fmt.Errorf("discover resources: %w", err)
	}
	if len(resources) == 0 {
		logger.Warnf("no resources discovered; nothing to do")
		return "no resources", nil
	}

	rendered, err := policy.BuildPolicies(resources, cfg, logger)
	if err != nil {
		return "", fmt.Errorf("build policies: %w", err)
	}

	fmsClient := fms.NewFromConfig(awsCfg)
	for _, p := range rendered {
		if err := fmsapply.UpsertPolicy(ctx, fmsClient, p, ouID, event.DryRun, logger); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("processed %d resource(s)", len(rendered)), nil
}

func loadPolicyConfig(ctx context.Context, awsCfg aws.Config, logger *util.Logger) (*policyconfig.PolicyConfig, error) {
	if param := os.Getenv("CONFIG_SSM_PARAM"); param != "" {
		cfg, err := loadConfigFromSSM(ctx, awsCfg, param)
		if err != nil {
			return nil, fmt.Errorf("load config from SSM %s: %w", param, err)
		}
		return cfg, nil
	}

	// Allow override via CONFIG_PATH; fall back to embedded config for Lambda packaging.
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		return policyconfig.Load(path)
	}
	cfg, err := policyconfig.LoadFromBytes(configs.EmbeddedPolicyVariants)
	if err != nil {
		return nil, err
	}

	// Optional tag key overrides from env.
	if v := os.Getenv("PRIMARY_TAG_KEY"); v != "" {
		cfg.TagKeys.Primary = v
	}
	if v := os.Getenv("SECONDARY_TAG_KEY"); v != "" {
		cfg.TagKeys.Secondary = v
	}

	if v := os.Getenv("DEFAULT_PRIMARY_RULES"); v != "" {
		if _, ok := cfg.RuleSets.Primary[v]; ok {
			cfg.Defaults.Primary = v
		} else {
			logger.Warnf("DEFAULT_PRIMARY_RULES %s not found in ruleSets.primary; keeping %s", v, cfg.Defaults.Primary)
		}
	}
	if v := os.Getenv("DEFAULT_SECONDARY_RULES"); v != "" {
		if _, ok := cfg.RuleSets.Secondary[v]; ok {
			cfg.Defaults.Secondary = v
		} else {
			logger.Warnf("DEFAULT_SECONDARY_RULES %s not found in ruleSets.secondary; keeping %s", v, cfg.Defaults.Secondary)
		}
	}

	return cfg, nil
}

func loadConfigFromSSM(ctx context.Context, awsCfg aws.Config, param string) (*policyconfig.PolicyConfig, error) {
	client := ssm.NewFromConfig(awsCfg)
	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(param),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	if out.Parameter == nil || out.Parameter.Value == nil {
		return nil, fmt.Errorf("parameter %s missing value", param)
	}
	return policyconfig.LoadFromBytes([]byte(*out.Parameter.Value))
}

func loadAWSConfig(ctx context.Context, region string) (awsCfg aws.Config, err error) {
	if region != "" {
		awsCfg, err = awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	} else {
		awsCfg, err = awsconfig.LoadDefaultConfig(ctx)
	}
	return
}
