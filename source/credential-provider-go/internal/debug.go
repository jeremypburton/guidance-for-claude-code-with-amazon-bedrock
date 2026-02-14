package internal

import (
	"fmt"
	"os"
)

// Debug controls whether debug messages are printed to stderr.
var Debug = true

// DebugPrint prints a message to stderr if debug mode is enabled.
func DebugPrint(format string, args ...interface{}) {
	if Debug {
		fmt.Fprintf(os.Stderr, "Debug: "+format+"\n", args...)
	}
}
