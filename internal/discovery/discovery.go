package discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/forkedpacket/aws-fms-secpolicy-learning/internal/util"
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
// AI EXTENSION: Create a DiscoverAPIGateways() function mirroring this pattern.
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
//
//	arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/demo-alb/abcd1234
func lbNameFromArn(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) == 0 {
		return arn
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}
