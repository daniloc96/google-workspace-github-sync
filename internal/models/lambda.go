package models

import "fmt"

// LambdaEvent is the input event for Lambda invocation.
type LambdaEvent struct {
	DryRun     *bool  `json:"dry_run,omitempty"`
	Source     string `json:"source,omitempty"`
	DetailType string `json:"detail-type,omitempty"`
}

// IsDryRun returns the effective dry-run setting.
func (e *LambdaEvent) IsDryRun(defaultValue bool) bool {
	if e != nil && e.DryRun != nil {
		return *e.DryRun
	}
	return defaultValue
}

// LambdaResponse is the output from Lambda invocation.
type LambdaResponse struct {
	StatusCode int         `json:"status_code"`
	Message    string      `json:"message"`
	Result     *SyncResult `json:"result,omitempty"`
}

// NewSuccessResponse creates a success response.
func NewSuccessResponse(result *SyncResult) *LambdaResponse {
	msg := fmt.Sprintf("Sync completed: %d actions", result.Summary.ActionsPlanned)
	if result.DryRun {
		msg = "[DRY RUN] " + msg
	}
	return &LambdaResponse{
		StatusCode: 200,
		Message:    msg,
		Result:     result,
	}
}

// NewErrorResponse creates an error response.
func NewErrorResponse(err error) *LambdaResponse {
	return &LambdaResponse{
		StatusCode: 500,
		Message:    err.Error(),
	}
}
