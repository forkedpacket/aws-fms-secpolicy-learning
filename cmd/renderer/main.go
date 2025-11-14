package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	policyconfig "github.com/forkedpacket/aws-fms-secpolicy-learning/internal/config"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/discovery"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/policy"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/util"
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

// run is separated from main() to make the workflow easier to test.
// It loads the desired policy config, optionally discovers resources, and renders the final JSON payload.
func run(ctx context.Context, logger *util.Logger) error {
	logger.Infof("loading policy config from %s", *flagConfig)
	cfg, err := policyconfig.Load(*flagConfig)
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

// loadAWSConfig wraps aws-sdk-go-v2's loader so that callers can optionally pin a region.
// Keeping this logic in one place makes it easier to add credentials/profile support later.
func loadAWSConfig(ctx context.Context, region string) (awsCfg aws.Config, err error) {
	if region != "" {
		awsCfg, err = awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	} else {
		awsCfg, err = awsconfig.LoadDefaultConfig(ctx)
	}
	return
}
