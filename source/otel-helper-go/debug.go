package main

import (
	"fmt"
	"os"
	"strings"
)

var (
	debugMode bool
	testMode  bool
)

// initDebug checks the DEBUG_MODE environment variable.
func initDebug() {
	val := strings.ToLower(os.Getenv("DEBUG_MODE"))
	switch val {
	case "true", "1", "yes", "y":
		debugMode = true
	}
}

// debugPrint writes a debug message to stderr if debug mode is enabled.
func debugPrint(format string, args ...interface{}) {
	if debugMode {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

// logWarning writes a warning message to stderr.
func logWarning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[WARNING] "+format+"\n", args...)
}

// logError writes an error message to stderr.
func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
}

// logInfo writes an info message to stderr if debug mode is enabled.
func logInfo(format string, args ...interface{}) {
	if debugMode {
		fmt.Fprintf(os.Stderr, "[INFO] "+format+"\n", args...)
	}
}
