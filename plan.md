# Plan: File-based debug logging for Go OTEL Helper

## Goal
When debug mode is enabled, write log output to a file instead of stderr.

## Design Decisions

**Log file path configuration**: Use a new environment variable `OTEL_HELPER_LOG_FILE`.
- If set, all log output goes to that file path instead of stderr.
- If unset, behavior is unchanged (logs to stderr as today).
- This keeps the change minimal and consistent with the existing `DEBUG_MODE` env var pattern.

**Log level routing**: Dual-destination for warnings and errors.
- `debugPrint` and `logInfo` write to the log file only (quiet in the terminal).
- `logWarning` and `logError` write to **both** the log file and stderr, so operators always see problems in the terminal even when file logging is active.
- When no log file is configured, all levels go to stderr as today (no behavior change).

## Changes

### 1. `debug.go` — Add file logging support

- Add a package-level `var logWriter io.Writer` initialized to `os.Stderr`.
- Add a `var logFile *os.File` for cleanup.
- Modify `initDebug()` to:
  1. Check `OTEL_HELPER_LOG_FILE` env var.
  2. If set, open the file (create/append mode, `0644` perms) and assign it to `logWriter`.
  3. Store the `*os.File` in `logFile` for later cleanup.
- Add a `closeDebug()` function that flushes and closes `logFile` if non-nil.
- Change `debugPrint` and `logInfo` to write to `logWriter` instead of hard-coded `os.Stderr`.
- Change `logWarning` and `logError` to write to **both** `logWriter` and `os.Stderr` when a log file is active (using a `logBoth()` helper), so warnings/errors are always visible in the terminal.

**Before:**
```go
var (
    debugMode bool
    testMode  bool
)

func initDebug() {
    val := strings.ToLower(os.Getenv("DEBUG_MODE"))
    switch val {
    case "true", "1", "yes", "y":
        debugMode = true
    }
}

func debugPrint(format string, args ...interface{}) {
    if debugMode {
        fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
    }
}
```

**After:**
```go
var (
    debugMode bool
    testMode  bool
    logWriter io.Writer = os.Stderr
    logFile   *os.File
)

func initDebug() {
    val := strings.ToLower(os.Getenv("DEBUG_MODE"))
    switch val {
    case "true", "1", "yes", "y":
        debugMode = true
    }

    if path := os.Getenv("OTEL_HELPER_LOG_FILE"); path != "" {
        f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
        if err != nil {
            fmt.Fprintf(os.Stderr, "[WARNING] Could not open log file %s: %v, falling back to stderr\n", path, err)
            return
        }
        logWriter = f
        logFile = f
    }
}

func closeDebug() {
    if logFile != nil {
        logFile.Close()
    }
}

// debugPrint and logInfo write to logWriter only (file when configured, stderr otherwise).
func debugPrint(format string, args ...interface{}) {
    if debugMode {
        fmt.Fprintf(logWriter, "[DEBUG] "+format+"\n", args...)
    }
}

func logInfo(format string, args ...interface{}) {
    if debugMode {
        fmt.Fprintf(logWriter, "[INFO] "+format+"\n", args...)
    }
}

// logWarning and logError always write to stderr.
// When a log file is active, they also write to the file for completeness.
func logWarning(format string, args ...interface{}) {
    msg := fmt.Sprintf("[WARNING] "+format+"\n", args...)
    fmt.Fprint(os.Stderr, msg)
    if logFile != nil {
        fmt.Fprint(logWriter, msg)
    }
}

func logError(format string, args ...interface{}) {
    msg := fmt.Sprintf("[ERROR] "+format+"\n", args...)
    fmt.Fprint(os.Stderr, msg)
    if logFile != nil {
        fmt.Fprint(logWriter, msg)
    }
}
```

### 2. `main.go` — Add cleanup call

- After `initDebug()`, add `defer closeDebug()`.

**Before:**
```go
func run() int {
    ...
    initDebug()
    ...
```

**After:**
```go
func run() int {
    ...
    initDebug()
    defer closeDebug()
    ...
```

Note: `defer` works in `run()` because `main()` calls `os.Exit(run())` — the defer executes before `run()` returns the exit code.

### 3. `debug_test.go` — Update tests

- Add tests for `OTEL_HELPER_LOG_FILE`:
  - **TestInitDebug_LogFile**: Set env var to a temp file, call `initDebug()`, verify `logWriter` points to the file, write a debug message, verify it appears in the file and NOT on stderr.
  - **TestInitDebug_LogFile_InvalidPath**: Set env var to an invalid path (e.g., `/nonexistent/dir/file.log`), verify fallback to stderr with no panic.
  - **TestCloseDebug**: Verify `closeDebug()` closes the file and is safe to call when `logFile` is nil.
  - **TestLogWarning_DualWrite**: With log file active, call `logWarning()`, verify the message appears in BOTH the log file and stderr.
  - **TestLogError_DualWrite**: Same as above for `logError()`.
  - **TestDebugPrint_FileOnly**: With log file active, call `debugPrint()`, verify message appears in the file but NOT on stderr.
- Existing tests remain unchanged (they don't set `OTEL_HELPER_LOG_FILE`, so behavior is identical).
- Add cleanup in tests: reset `logWriter = os.Stderr` and `logFile = nil` in each test teardown to avoid cross-test contamination.

### 4. `OTEL_HELPER.md` — Update documentation

- Add `OTEL_HELPER_LOG_FILE` to the Environment Variables table.
- Update the Security Considerations section noting that log files may contain redacted JWT payloads when debug mode is active.

## Files Modified

| File | Change |
|------|--------|
| `source/otel-helper-go/debug.go` | Add `logWriter`, `logFile`, `closeDebug()`; update all log functions |
| `source/otel-helper-go/main.go` | Add `defer closeDebug()` after `initDebug()` |
| `source/otel-helper-go/debug_test.go` | Add file logging tests, cleanup helpers |
| `assets/docs/OTEL_HELPER.md` | Document new env var |

## What Does NOT Change

- Python implementation (reference only, not shipped).
- Normal-mode behavior (no `OTEL_HELPER_LOG_FILE` set = identical to today).
- stdout output (JSON headers) — completely unaffected.
- Test mode (`--test`) output — still goes to stdout.
- The `DEBUG_MODE` env var — still controls whether debug/info messages are emitted at all.
