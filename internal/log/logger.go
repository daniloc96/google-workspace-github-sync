package log

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// ANSI color codes.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

// PrettyFormatter formats log entries in a human-readable way for terminal output.
type PrettyFormatter struct{}

// Format renders a logrus entry as a pretty, human-readable line.
func (f *PrettyFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := entry.Time.Format("15:04:05")

	var levelIcon string
	var levelColor string
	switch entry.Level {
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		levelIcon = "✗"
		levelColor = colorRed
	case logrus.WarnLevel:
		levelIcon = "⚠"
		levelColor = colorYellow
	case logrus.InfoLevel:
		levelIcon = "•"
		levelColor = colorGreen
	case logrus.DebugLevel, logrus.TraceLevel:
		levelIcon = "·"
		levelColor = colorGray
	}

	msg := entry.Message

	// Build key=value context string from fields (sorted for consistency).
	var fieldParts []string
	keys := make([]string, 0, len(entry.Data))
	for k := range entry.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := entry.Data[k]
		fieldParts = append(fieldParts, fmt.Sprintf("%s%s%s=%v", colorCyan, k, colorReset, v))
	}

	var fieldsStr string
	if len(fieldParts) > 0 {
		fieldsStr = " " + strings.Join(fieldParts, " ")
	}

	line := fmt.Sprintf("%s%s%s %s%s%s %s%s\n",
		colorGray, timestamp, colorReset,
		levelColor, levelIcon, colorReset,
		msg,
		fieldsStr,
	)
	return []byte(line), nil
}

// NewLogger creates a configured logrus logger.
func NewLogger(level string, format string) *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	setFormatter(logger, format)
	setLevel(logger, level)
	return logger
}

// Configure sets output, format, and level on an existing logger.
func Configure(logger *logrus.Logger, out io.Writer, level string, format string) {
	if out != nil {
		logger.SetOutput(out)
	}
	setFormatter(logger, format)
	setLevel(logger, level)
}

func setFormatter(logger *logrus.Logger, format string) {
	switch format {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{})
	case "pretty":
		logger.SetFormatter(&PrettyFormatter{})
	default:
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "15:04:05",
		})
	}
}

func setLevel(logger *logrus.Logger, level string) {
	parsed, err := logrus.ParseLevel(level)
	if err != nil {
		parsed = logrus.InfoLevel
	}
	logger.SetLevel(parsed)
}
