package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

var (
	debugMode bool
	testMode  bool
	logWriter io.Writer = os.Stderr
	logFile   *os.File
)

// initDebug checks the DEBUG_MODE environment variable and optionally
// opens a log file specified by OTEL_HELPER_LOG_FILE.
func initDebug() {
	val := strings.ToLower(os.Getenv("DEBUG_MODE"))
	switch val {
	case "true", "1", "yes", "y":
		debugMode = true
	}

	if path := os.Getenv("OTEL_HELPER_LOG_FILE"); path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[WARNING] Could not open log file %s: %v, falling back to stderr\n", path, err)
			return
		}
		logWriter = f
		logFile = f
	}
}

// closeDebug closes the log file if one was opened and resets the writer to stderr.
func closeDebug() {
	if logFile != nil {
		logFile.Close()
		logFile = nil
		logWriter = os.Stderr
	}
}

// timestamp returns the current time formatted for log messages.
func timestamp() string {
	return time.Now().Format("2006-01-02T15:04:05.000Z07:00")
}

// debugPrint writes a debug message to logWriter if debug mode is enabled.
func debugPrint(format string, args ...interface{}) {
	if debugMode {
		fmt.Fprintf(logWriter, "%s [DEBUG] "+format+"\n", append([]interface{}{timestamp()}, args...)...)
	}
}

// logWarning writes a warning message to stderr.
// When a log file is active, also writes to the log file.
func logWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf("%s [WARNING] "+format+"\n", append([]interface{}{timestamp()}, args...)...)
	fmt.Fprint(os.Stderr, msg)
	if logFile != nil {
		fmt.Fprint(logWriter, msg)
	}
}

// logError writes an error message to stderr.
// When a log file is active, also writes to the log file.
func logError(format string, args ...interface{}) {
	msg := fmt.Sprintf("%s [ERROR] "+format+"\n", append([]interface{}{timestamp()}, args...)...)
	fmt.Fprint(os.Stderr, msg)
	if logFile != nil {
		fmt.Fprint(logWriter, msg)
	}
}

// logInfo writes an info message to logWriter if debug mode is enabled.
func logInfo(format string, args ...interface{}) {
	if debugMode {
		fmt.Fprintf(logWriter, "%s [INFO] "+format+"\n", append([]interface{}{timestamp()}, args...)...)
	}
}
