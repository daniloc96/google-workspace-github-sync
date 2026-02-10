package metrics

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

type mockCloudWatch struct {
	input *cloudwatch.PutMetricDataInput
}

func (m *mockCloudWatch) PutMetricData(ctx context.Context, params *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	m.input = params
	return &cloudwatch.PutMetricDataOutput{}, nil
}

func TestEmitSummary(t *testing.T) {
	client := &mockCloudWatch{}
	emitter := &Emitter{client: client, namespace: "TestNamespace"}

	summary := models.SyncSummary{
		ActionsPlanned:  3,
		ActionsExecuted: 2,
		ActionsFailed:   1,
		Invited:         1,
		Removed:         1,
		RoleUpdated:     0,
		Skipped:         1,
	}

	err := emitter.EmitSummary(context.Background(), summary, []string{"err1"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client.input == nil {
		t.Fatalf("expected metric input to be sent")
	}
	if *client.input.Namespace != "TestNamespace" {
		t.Fatalf("expected namespace TestNamespace, got %s", aws.ToString(client.input.Namespace))
	}
	if len(client.input.MetricData) != 8 {
		t.Fatalf("expected 8 metrics, got %d", len(client.input.MetricData))
	}
}
