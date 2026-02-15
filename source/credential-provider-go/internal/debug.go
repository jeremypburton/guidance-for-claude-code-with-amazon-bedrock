package internal

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Debug controls whether debug messages are emitted via DebugPrint.
// Enabled by setting the DEBUG_MODE environment variable to true, 1, yes, or y.
// Output goes to LogWriter, which defaults to stderr but is redirected
// to a file when CREDENTIAL_PROCESS_LOG_FILE is set.
var Debug bool

var (
	// LogWriter is the destination for debug log output.
	// Defaults to os.Stderr; redirected to a file when CREDENTIAL_PROCESS_LOG_FILE is set.
	LogWriter io.Writer = os.Stderr

	// LogFile holds the open log file handle for cleanup, or nil if logging to stderr.
	LogFile *os.File
)

// InitDebug checks DEBUG_MODE to enable debug output, and
// CREDENTIAL_PROCESS_LOG_FILE to redirect output to a file.
func InitDebug() {
	val := strings.ToLower(os.Getenv("DEBUG_MODE"))
	switch val {
	case "true", "1", "yes", "y":
		Debug = true
	}

	if path := os.Getenv("CREDENTIAL_PROCESS_LOG_FILE"); path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[WARNING] Could not open log file %s: %v, falling back to stderr\n", path, err)
			return
		}
		LogWriter = f
		LogFile = f
	}
}

// CloseDebug closes the log file if one was opened and resets the writer to stderr.
func CloseDebug() {
	if LogFile != nil {
		LogFile.Close()
		LogFile = nil
		LogWriter = os.Stderr
	}
}

// timestamp returns the current time formatted for log messages.
func timestamp() string {
	return time.Now().Format("2006-01-02T15:04:05.000Z07:00")
}

// DebugPrint writes a debug message to LogWriter if debug mode is enabled.
func DebugPrint(format string, args ...interface{}) {
	if Debug {
		fmt.Fprintf(LogWriter, "%s Debug: "+format+"\n", append([]interface{}{timestamp()}, args...)...)
	}
}

// StatusPrint writes a user-facing message (errors, status, informational) to stderr.
// When a log file is active, also writes to the log file for completeness.
// Unlike DebugPrint, this is not gated on the Debug flag — messages always appear.
func StatusPrint(format string, args ...interface{}) {
	msg := fmt.Sprintf("%s "+format, append([]interface{}{timestamp()}, args...)...)
	fmt.Fprint(os.Stderr, msg)
	if LogFile != nil {
		fmt.Fprint(LogWriter, msg)
	}
}
