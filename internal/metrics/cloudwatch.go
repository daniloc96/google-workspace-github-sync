package metrics

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

// CloudWatchAPI defines the CloudWatch client interface used for metrics.
type CloudWatchAPI interface {
	PutMetricData(ctx context.Context, params *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error)
}

// Emitter sends sync metrics to CloudWatch.
type Emitter struct {
	client    CloudWatchAPI
	namespace string
}

// NewEmitter creates a CloudWatch metrics emitter.
func NewEmitter(cfg aws.Config, namespace string) *Emitter {
	return &Emitter{
		client:    cloudwatch.NewFromConfig(cfg),
		namespace: namespace,
	}
}

// EmitSummary publishes sync metrics to CloudWatch.
func (e *Emitter) EmitSummary(ctx context.Context, summary models.SyncSummary, errors []string) error {
	metrics := []types.MetricDatum{
		metricDatum("ActionsPlanned", summary.ActionsPlanned),
		metricDatum("ActionsExecuted", summary.ActionsExecuted),
		metricDatum("ActionsFailed", summary.ActionsFailed),
		metricDatum("Invited", summary.Invited),
		metricDatum("Removed", summary.Removed),
		metricDatum("RoleUpdated", summary.RoleUpdated),
		metricDatum("Skipped", summary.Skipped),
		metricDatum("Errors", len(errors)),
	}

	_, err := e.client.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
		Namespace:  aws.String(e.namespace),
		MetricData: metrics,
	})
	return err
}

func metricDatum(name string, value int) types.MetricDatum {
	return types.MetricDatum{
		MetricName: aws.String(name),
		Unit:       types.StandardUnitCount,
		Value:      aws.Float64(float64(value)),
	}
}
