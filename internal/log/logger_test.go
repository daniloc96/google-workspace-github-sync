package log

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestJSONFormatterOutput(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := logrus.New()
	Configure(logger, buf, "info", "json")

	logger.Info("test message")

	var payload map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got error: %v", err)
	}
	if payload["msg"] != "test message" {
		t.Fatalf("expected msg field to be 'test message', got %v", payload["msg"])
	}
}
