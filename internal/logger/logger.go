package logger

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Level represents log severity.
type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

func log(level Level, msg string, fields map[string]any) {
	ts := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s %s", ts, level, msg)
	for k, v := range fields {
		fmt.Fprintf(&sb, " %s=%v", k, v)
	}
	fmt.Fprintln(os.Stdout, sb.String())
}

// Info logs an informational message.
func Info(msg string, fields map[string]any) { log(LevelInfo, msg, fields) }

// Warn logs a warning message.
func Warn(msg string, fields map[string]any) { log(LevelWarn, msg, fields) }

// Error logs an error message.
func Error(msg string, fields map[string]any) { log(LevelError, msg, fields) }
