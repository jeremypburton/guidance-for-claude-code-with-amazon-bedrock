package internal

import (
	"fmt"
	"io"
	"os"
)

// Debug controls whether debug messages are printed to stderr.
var Debug = true

var (
	// LogWriter is the destination for debug log output.
	// Defaults to os.Stderr; redirected to a file when CREDENTIAL_PROCESS_LOG_FILE is set.
	LogWriter io.Writer = os.Stderr

	// LogFile holds the open log file handle for cleanup, or nil if logging to stderr.
	LogFile *os.File
)

// InitDebug checks CREDENTIAL_PROCESS_LOG_FILE and opens the log file if set.
func InitDebug() {
	if path := os.Getenv("CREDENTIAL_PROCESS_LOG_FILE"); path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not open log file %s: %v, falling back to stderr\n", path, err)
			return
		}
		LogWriter = f
		LogFile = f
	}
}

// CloseDebug closes the log file if one was opened.
func CloseDebug() {
	if LogFile != nil {
		LogFile.Close()
	}
}

// DebugPrint writes a debug message to LogWriter if debug mode is enabled.
func DebugPrint(format string, args ...interface{}) {
	if Debug {
		fmt.Fprintf(LogWriter, "Debug: "+format+"\n", args...)
	}
}

// ErrorPrint writes an error/status message to stderr.
// When a log file is active, also writes to the log file for completeness.
func ErrorPrint(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprint(os.Stderr, msg)
	if LogFile != nil {
		fmt.Fprint(LogWriter, msg)
	}
}
