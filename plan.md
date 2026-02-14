# Plan: File-based debug logging for Credential Process (Go)

## Goal
When a log file is configured, write debug output to a file instead of stderr — matching the pattern already implemented in the OTEL Helper.

## Current State

The credential process has a single logging function in `internal/debug.go`:

```go
var Debug = true

func DebugPrint(format string, args ...interface{}) {
    if Debug {
        fmt.Fprintf(os.Stderr, "Debug: "+format+"\n", args...)
    }
}
```

`DebugPrint` is called from **8 packages**: `main.go`, `config/`, `credentials/`, `federation/`, `locking/`, `monitoring/`, `quota/`, and `internal/` itself. All callers use `internal.DebugPrint(...)`.

Additionally, `main.go` has ~12 direct `fmt.Fprintf(os.Stderr, "Error: ...")` calls for user-facing error output. These are **not** debug messages — they're normal error reporting that should always be visible.

## Design Decisions

**Log file path**: New environment variable `CREDENTIAL_PROCESS_LOG_FILE`.
- If set, `DebugPrint` writes to the file instead of stderr.
- If unset, behavior is unchanged (debug output goes to stderr as today).
- Consistent with the `OTEL_HELPER_LOG_FILE` pattern.

**Log level routing — dual-write for user-facing errors**:
- `DebugPrint` writes to the log file only (quiet terminal) — same as `debugPrint`/`logInfo` in the OTEL Helper.
- A new `ErrorPrint` function writes to **both** stderr and the log file — same as `logWarning`/`logError` in the OTEL Helper. This replaces the direct `fmt.Fprintf(os.Stderr, ...)` calls in `main.go` that report errors.
- When no log file is configured, everything goes to stderr as today.

**Why add `ErrorPrint`?**: Without it, error messages from `main.go` would only appear on stderr and never reach the log file, making debugging incomplete. By routing them through `ErrorPrint`, the log file captures the full picture.

## Changes

### 1. `internal/debug.go` — Add file logging + `ErrorPrint`

- Add `LogWriter io.Writer` (exported, initialized to `os.Stderr`).
- Add `LogFile *os.File` for cleanup.
- Add `InitDebug()` — checks `CREDENTIAL_PROCESS_LOG_FILE` env var, opens file.
- Add `CloseDebug()` — closes the file if open.
- Update `DebugPrint` to write to `LogWriter` instead of `os.Stderr`.
- Add `ErrorPrint` — always writes to stderr, additionally to log file when active.

**Before:**
```go
package internal

import (
    "fmt"
    "os"
)

var Debug = true

func DebugPrint(format string, args ...interface{}) {
    if Debug {
        fmt.Fprintf(os.Stderr, "Debug: "+format+"\n", args...)
    }
}
```

**After:**
```go
package internal

import (
    "fmt"
    "io"
    "os"
)

var Debug = true

var (
    LogWriter io.Writer = os.Stderr
    LogFile   *os.File
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
// When a log file is active, also writes to the log file.
func ErrorPrint(format string, args ...interface{}) {
    msg := fmt.Sprintf(format, args...)
    fmt.Fprint(os.Stderr, msg)
    if LogFile != nil {
        fmt.Fprint(LogWriter, msg)
    }
}
```

Note: Variables and functions are **exported** (capitalized) because they live in the `internal` package and are called from `main`, `config`, `credentials`, etc.

### 2. `main.go` — Add init/cleanup + use `ErrorPrint`

**Add lifecycle calls** at the top of `run()`:
```go
func run() int {
    // ... flag parsing ...
    flag.Parse()

    internal.InitDebug()
    defer internal.CloseDebug()

    // ... rest of run() ...
```

**Replace direct stderr writes** with `internal.ErrorPrint`. There are ~12 instances in `main.go`:

| Line | Current | Replacement |
|------|---------|-------------|
| 61 | `fmt.Fprintf(os.Stderr, "Error: %v\n", err)` | `internal.ErrorPrint("Error: %v\n", err)` |
| 68 | `fmt.Fprintf(os.Stderr, "Error: %v\n", err)` | `internal.ErrorPrint("Error: %v\n", err)` |
| 73 | `fmt.Fprintf(os.Stderr, "Error: unknown provider type '%s'\n", providerType)` | `internal.ErrorPrint("Error: unknown provider type '%s'\n", providerType)` |
| 83-88 | Cache clearing status messages | `internal.ErrorPrint(...)` |
| 116 | Credentials expired message | `internal.ErrorPrint(...)` |
| 119 | Credentials valid message | `internal.ErrorPrint(...)` |
| 126 | refresh-if-needed error | `internal.ErrorPrint(...)` |
| 200 | Auth error | `internal.ErrorPrint(...)` |
| 219 | Federation error | `internal.ErrorPrint(...)` |

These all keep their existing format strings — only the destination function changes.

### 3. `internal/debug_test.go` — New test file

Create a new test file (none currently exists for `internal/debug.go`):

- **TestDebugPrint_WritesToLogWriter**: Set `Debug = true`, set `LogWriter` to a buffer, call `DebugPrint`, verify output.
- **TestDebugPrint_DisabledSkips**: Set `Debug = false`, verify no output.
- **TestInitDebug_LogFile**: Set env var to temp file, call `InitDebug()`, verify `LogFile` is non-nil and `LogWriter` points to file.
- **TestInitDebug_InvalidPath**: Set env var to invalid path, verify fallback to stderr.
- **TestInitDebug_NoEnvVar**: Verify no-op when env var unset.
- **TestCloseDebug_NilSafe**: Call `CloseDebug()` with `LogFile = nil`, verify no panic.
- **TestErrorPrint_DualWrite**: With log file active, call `ErrorPrint`, verify message in both stderr and file.
- **TestDebugPrint_FileOnly**: With log file active, call `DebugPrint`, verify in file but NOT on stderr.

Each test uses a cleanup helper to reset `LogWriter = os.Stderr`, `LogFile = nil`, `Debug = true`.

### 4. Documentation update

Add a note about `CREDENTIAL_PROCESS_LOG_FILE` to `assets/docs/OTEL_HELPER.md` in a new "Credential Process Logging" subsection under Configuration Reference, or to the existing MONITORING.md if more appropriate.

## Files Modified

| File | Change |
|------|--------|
| `source/credential-provider-go/internal/debug.go` | Add `LogWriter`, `LogFile`, `InitDebug()`, `CloseDebug()`, `ErrorPrint()`; update `DebugPrint` |
| `source/credential-provider-go/main.go` | Add `InitDebug()`/`CloseDebug()` lifecycle; replace `fmt.Fprintf(os.Stderr, ...)` with `internal.ErrorPrint(...)` |
| `source/credential-provider-go/internal/debug_test.go` | New file — 8 tests |
| `assets/docs/OTEL_HELPER.md` or `assets/docs/MONITORING.md` | Document `CREDENTIAL_PROCESS_LOG_FILE` env var |

## What Does NOT Change

- All other packages (`config/`, `credentials/`, `federation/`, `locking/`, `monitoring/`, `quota/`) continue calling `internal.DebugPrint(...)` unchanged — it just routes to a different writer.
- `internal.Debug` flag behavior (controls whether `DebugPrint` emits anything) stays the same.
- stdout output (JSON credentials) — completely unaffected.
- Python credential provider implementation — not modified.
- No log file configured = identical behavior to today.
