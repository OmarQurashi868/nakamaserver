package logger

import (
	"fmt"
	"os"
	"sort"
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

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

func levelColor(level Level) string {
	switch level {
	case LevelInfo:
		return colorGreen
	case LevelWarn:
		return colorYellow
	case LevelError:
		return colorRed
	default:
		return colorReset
	}
}

func log(level Level, msg string, fields map[string]any) {
	ts := time.Now().Format("2006-01-02 15:04:05")

	// Sort field keys for consistent output
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder

	// Timestamp (gray)
	fmt.Fprintf(&sb, "%s%s%s ", colorGray, ts, colorReset)

	// Level badge (colored + bold)
	fmt.Fprintf(&sb, "%s%s%-5s%s ", levelColor(level), colorBold, string(level), colorReset)

	// Message (cyan for readability)
	fmt.Fprintf(&sb, "%s%s%s", colorCyan, msg, colorReset)

	// Fields
	for _, k := range keys {
		fmt.Fprintf(&sb, " %s%s%s=%v", colorGray, k, colorReset, fields[k])
	}

	fmt.Fprintln(os.Stdout, sb.String())
}

// Info logs an informational message.
func Info(msg string, fields map[string]any) { log(LevelInfo, msg, fields) }

// Warn logs a warning message.
func Warn(msg string, fields map[string]any) { log(LevelWarn, msg, fields) }

// Error logs an error message.
func Error(msg string, fields map[string]any) { log(LevelError, msg, fields) }
