# OTEL Helper - Technical Documentation

The OTEL Helper is a lightweight binary that extracts user identity attributes from JWT tokens and outputs them as HTTP headers. Claude Code calls this binary before each telemetry export, attaching the returned headers to OTLP requests so the OTEL Collector can attribute metrics to individual users, teams, and departments.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Sequence Diagrams](#sequence-diagrams)
- [Implementations](#implementations)
- [Data Flow](#data-flow)
- [JWT Claim Extraction](#jwt-claim-extraction)
- [Header Mapping](#header-mapping)
- [OIDC Provider Detection](#oidc-provider-detection)
- [Infrastructure Integration](#infrastructure-integration)
- [Build and Distribution](#build-and-distribution)
- [Configuration Reference](#configuration-reference)
- [Security Considerations](#security-considerations)

## Overview

The OTEL Helper serves one purpose: convert a JWT monitoring token into a flat JSON object of HTTP headers that the OTEL Collector understands. This enables user-level metric attribution in CloudWatch without requiring Claude Code to understand JWT internals.

```
JWT Token --> [OTEL Helper] --> {"x-user-email": "alice@acme.com", "x-user-id": "a1b2c3...", ...}
```

Two implementations exist:
- **Go** (`source/otel-helper-go/`) - Compiled binary, used in production distribution packages. Zero external dependencies (stdlib only).
- **Python** (`source/otel_helper/`) - Reference implementation, useful for development and debugging.

Both produce identical output for the same input token.

## Architecture

```
+------------------+     +-------------------+     +------------------+     +-------------------+
|                  |     |                   |     |                  |     |                   |
|   Claude Code    |---->|   OTEL Helper     |---->|  OTEL Collector  |---->|   CloudWatch      |
|   (client)       |     |   (binary)        |     |  (ECS Fargate)   |     |   (metrics/logs)  |
|                  |     |                   |     |                  |     |                   |
+------------------+     +-------------------+     +------------------+     +-------------------+
        |                     |       |                    |
        |                     |       |                    |
        v                     v       v                    v
   settings.json         JWT Token   JSON Headers     Dimensions:
   otelHeadersHelper     from env    to stdout        user.email,
   = ~/claude-code-      or cred-                     department,
     with-bedrock/       process                      team.id, etc.
     otel-helper
```

### Component Responsibilities

| Component | Role |
|-----------|------|
| **Claude Code** | Calls `otelHeadersHelper` binary, attaches returned headers to OTLP metric exports |
| **OTEL Helper** | Decodes JWT, extracts user attributes, formats as HTTP headers JSON |
| **Credential Process** | Provides the JWT monitoring token when not available in the environment |
| **OTEL Collector** | Receives OTLP metrics + headers, maps headers to resource attributes, exports to CloudWatch |
| **CloudWatch** | Stores metrics with user/org dimensions for dashboards and alerting |

## Sequence Diagrams

### Primary Flow: Telemetry Export with User Attribution

```
Claude Code           OTEL Helper          Credential Process       OTEL Collector       CloudWatch
    |                     |                       |                       |                    |
    |  exec(otel-helper)  |                       |                       |                    |
    |-------------------->|                       |                       |                    |
    |                     |                       |                       |                    |
    |                     | check env var          |                       |                    |
    |                     | CLAUDE_CODE_           |                       |                    |
    |                     | MONITORING_TOKEN       |                       |                    |
    |                     |-------+               |                       |                    |
    |                     |       | found?         |                       |                    |
    |                     |<------+               |                       |                    |
    |                     |                       |                       |                    |
    |                     |  [if not found]        |                       |                    |
    |                     |  --get-monitoring-token |                       |                    |
    |                     |----------------------->|                       |                    |
    |                     |                       |                       |                    |
    |                     |  JWT token (stdout)    |                       |                    |
    |                     |<-----------------------|                       |                    |
    |                     |                       |                       |                    |
    |                     | decode JWT payload     |                       |                    |
    |                     |-------+               |                       |                    |
    |                     |       | base64 decode  |                       |                    |
    |                     |       | parse JSON     |                       |                    |
    |                     |<------+               |                       |                    |
    |                     |                       |                       |                    |
    |                     | extract user info      |                       |                    |
    |                     |-------+               |                       |                    |
    |                     |       | email, sub,    |                       |                    |
    |                     |       | dept, team...  |                       |                    |
    |                     |<------+               |                       |                    |
    |                     |                       |                       |                    |
    |                     | format as headers      |                       |                    |
    |                     |-------+               |                       |                    |
    |                     |       | x-user-email,  |                       |                    |
    |                     |       | x-user-id ...  |                       |                    |
    |                     |<------+               |                       |                    |
    |                     |                       |                       |                    |
    |  JSON to stdout     |                       |                       |                    |
    |<--------------------|                       |                       |                    |
    |                     |                       |                       |                    |
    |  OTLP metrics + HTTP headers                 |                       |                    |
    |--------------------------------------------->|                       |                    |
    |                                              |                       |                    |
    |                                              | attributes processor   |                    |
    |                                              | maps headers to        |                    |
    |                                              | resource attributes    |                    |
    |                                              |-------+               |                    |
    |                                              |       |               |                    |
    |                                              |<------+               |                    |
    |                                              |                       |                    |
    |                                              |  EMF metrics with      |                    |
    |                                              |  user dimensions       |                    |
    |                                              |----------------------->|                    |
    |                                              |                       |                    |
```

### Token Acquisition Flow

```
OTEL Helper                  Environment                Credential Process
    |                            |                            |
    | getenv(CLAUDE_CODE_        |                            |
    |   MONITORING_TOKEN)        |                            |
    |--------------------------->|                            |
    |                            |                            |
    |  [token found]             |                            |
    |<-- "eyJhbGc..." ----------|                            |
    |  --> use token directly    |                            |
    |                            |                            |
    |  [token NOT found]         |                            |
    |<-- "" --------------------|                            |
    |                            |                            |
    | getenv(AWS_PROFILE)        |                            |
    |--------------------------->|                            |
    |<-- "ClaudeCode" ----------|                            |
    |                            |                            |
    | exec: credential-process   |                            |
    |   --profile ClaudeCode     |                            |
    |   --get-monitoring-token   |                            |
    |--------------------------------------------------->|
    |                            |                            |
    |                            |              reads cached  |
    |                            |              token or      |
    |                            |              triggers auth |
    |                            |                            |
    |<-- JWT token (stdout) -----------------------------|
    |                            |                            |
    | [timeout: 300s]            |                            |
    |                            |                            |
```

### JWT Decode and Attribute Extraction Flow

```
OTEL Helper
    |
    | Input: "xxxxx.eyJlbWFpbCI6ImFsaWNlQGFjbWUuY29tIi...xxxxx"
    |
    | 1. Split on "."
    |-------+
    |       |  parts[0] = header (ignored)
    |       |  parts[1] = payload (base64url)
    |       |  parts[2] = signature (ignored)
    |<------+
    |
    | 2. Base64URL decode parts[1]
    |-------+
    |       |  Replace: - -> +, _ -> /
    |       |  Add padding: ==
    |       |  Decode to bytes
    |       |  Parse as JSON
    |<------+
    |
    | 3. Extract user attributes with fallback chains
    |-------+
    |       |  email:      claims.email || claims.preferred_username || claims.mail || "unknown@example.com"
    |       |  user_id:    SHA256(claims.sub) formatted as UUID
    |       |  username:   claims["cognito:username"] || claims.username || claims.preferred_username || email_prefix
    |       |  org:        detectProvider(claims.iss)  -->  "okta" | "auth0" | "azure" | "jc_org" | "amazon-internal"
    |       |  department: claims.department || claims.dept || claims.division || "unspecified"
    |       |  team:       claims.team || claims.groups[0] || "default-team"
    |       |  cost_center: claims.cost_center || claims.costCenter || "general"
    |       |  manager:    claims.manager || claims.manager_email || "unassigned"
    |       |  location:   claims.location || claims.office_location || "remote"
    |       |  role:       claims.role || claims.job_title || "user"
    |       |  company:    claims.company || (omitted)
    |<------+
    |
    | 4. Map to HTTP headers
    |-------+
    |       |  email       -> x-user-email
    |       |  user_id     -> x-user-id
    |       |  username    -> x-user-name
    |       |  department  -> x-department
    |       |  team        -> x-team-id
    |       |  cost_center -> x-cost-center
    |       |  org         -> x-organization
    |       |  location    -> x-location
    |       |  role        -> x-role
    |       |  manager     -> x-manager
    |       |  company     -> x-company  (only if non-empty)
    |<------+
    |
    | 5. Output JSON to stdout
    |
    | {"x-user-email":"alice@acme.com","x-user-id":"a1b2c3d4-...","x-department":"engineering",...}
    |
```

### OTEL Collector Processing Flow

```
OTLP Request                  Attributes Processor          Resource Processor         CloudWatch EMF Exporter
    |                               |                             |                            |
    | HTTP headers:                 |                             |                            |
    |  x-user-email: alice@...      |                             |                            |
    |  x-department: engineering    |                             |                            |
    |  x-team-id: platform          |                             |                            |
    |  ...                          |                             |                            |
    |                               |                             |                            |
    | Metric data:                  |                             |                            |
    |  claude_code.token.usage=1500 |                             |                            |
    |------------------------------>|                             |                            |
    |                               |                             |                            |
    |                               | Map headers to attributes:  |                            |
    |                               |  metadata.x-user-email      |                            |
    |                               |    -> user.email            |                            |
    |                               |  metadata.x-department      |                            |
    |                               |    -> department            |                            |
    |                               |  metadata.x-team-id         |                            |
    |                               |    -> team.id               |                            |
    |                               |  (10 header mappings)       |                            |
    |                               |                             |                            |
    |                               |  Metric with attributes:    |                            |
    |                               |--------------------------->|                            |
    |                               |                             |                            |
    |                               |                             | Add account/env:           |
    |                               |                             |  aws.account_id = 123...   |
    |                               |                             |  deployment.environment     |
    |                               |                             |    = production             |
    |                               |                             |                            |
    |                               |                             | Batch (60s window):        |
    |                               |                             |--------------------------->|
    |                               |                             |                            |
    |                               |                             |                            | Write to CloudWatch:
    |                               |                             |                            |  Namespace: ClaudeCode
    |                               |                             |                            |  Log Group:
    |                               |                             |                            |    /aws/claude-code/metrics
    |                               |                             |                            |  Dimensions:
    |                               |                             |                            |    [department, OTelLib]
    |                               |                             |                            |    [team.id, OTelLib]
    |                               |                             |                            |    [cost_center, OTelLib]
    |                               |                             |                            |    [model, OTelLib]
    |                               |                             |                            |    ...
```

## Implementations

### Go Implementation (Production)

Located in `source/otel-helper-go/`. This is the version compiled and distributed to end users.

| File | Purpose |
|------|---------|
| `main.go` | Entry point, CLI flag parsing, orchestrates the flow |
| `jwt.go` | `decodeJWTPayload()` - Base64URL decoding with sensitive field redaction |
| `userinfo.go` | `extractUserInfo()` - Claim extraction with provider-specific fallback chains; `UserInfo` struct |
| `provider.go` | `detectProvider()` - OIDC provider detection from issuer URL |
| `token.go` | `getTokenViaCredentialProcess()` - Calls credential-process binary with 5-min timeout |
| `headers.go` | `formatAsHeaders()` - Maps `UserInfo` fields to `x-*` HTTP header names |
| `debug.go` | Logging utilities, `DEBUG_MODE` / `OTEL_HELPER_LOG_FILE` initialization, file log lifecycle |

**Key properties:**
- Zero external dependencies (Go stdlib only, per `go.mod`)
- Cross-compiled for 5 platforms: macOS ARM64/Intel, Linux x64/ARM64, Windows
- Returns exit code 1 on failure (no token, decode error), exit code 0 on success

### Python Implementation (Reference)

Located in `source/otel_helper/__main__.py`. Single-file implementation (384 lines).

Uses only Python stdlib: `base64`, `hashlib`, `json`, `subprocess`, `os`, `argparse`, `logging`, `urllib.parse`.

Functionally identical to the Go version. Useful for debugging with `--test` or `--verbose` flags.

## Data Flow

### End-to-End Path

```
1. Credential Provider authenticates user via OIDC
         |
         v
2. JWT ID token cached in environment (CLAUDE_CODE_MONITORING_TOKEN)
   or retrievable via credential-process --get-monitoring-token
         |
         v
3. Claude Code calls otel-helper binary (configured via otelHeadersHelper in settings.json)
         |
         v
4. OTEL Helper decodes JWT, extracts claims, outputs JSON headers to stdout
         |
         v
5. Claude Code attaches headers to OTLP HTTP requests
         |
         v
6. ALB forwards to OTEL Collector (ECS Fargate, port 4318)
         |
         v
7. Collector's attributes processor maps x-* headers to resource attributes
         |
         v
8. Collector's resource processor adds aws.account_id, deployment.environment
         |
         v
9. awsemf exporter writes to CloudWatch (namespace: ClaudeCode, log group: /aws/claude-code/metrics)
         |
         v
10. CloudWatch Dashboard visualizes metrics with user/team/department dimensions
```

### Metric Types Flowing Through This Path

| Metric | Description |
|--------|-------------|
| `claude_code.token.usage` | Input/output token counts per request |
| `claude_code.cost.usage` | Estimated cost based on token consumption |
| `claude_code.session.count` | Active session tracking |
| `claude_code.active_time.total` | Time actively using Claude Code |
| `claude_code.code_edit_tool.decision` | Code editing accept/reject decisions |
| `claude_code.lines_of_code.count` | Lines of code changed |

Each metric carries the user attribution headers, enabling per-user and per-team breakdowns in CloudWatch.

## JWT Claim Extraction

The OTEL Helper extracts user attributes from JWT claims using provider-specific fallback chains. This handles differences between OIDC providers (Cognito, Okta, Auth0, Azure AD, JumpCloud).

### Claim Lookup Order

| Attribute | Claims checked (in order) | Default |
|-----------|---------------------------|---------|
| **email** | `email`, `preferred_username`, `mail` | `unknown@example.com` |
| **user_id** | `sub`, `user_id` (then SHA256 hashed, formatted as UUID) | `""` |
| **username** | `cognito:username`, `username`, `preferred_username`, `upn`, `name`, email prefix | email prefix |
| **organization** | Derived from `iss` via provider detection | `amazon-internal` |
| **department** | `department`, `dept`, `division`, `organizationalUnit` | `unspecified` |
| **team** | `team`, `team_id`, `group`, first 3 from `groups[]` | `default-team` |
| **cost_center** | `cost_center`, `costCenter`, `cost_code`, `costcenter` | `general` |
| **manager** | `manager`, `manager_email`, `managerId` | `unassigned` |
| **location** | `location`, `office_location`, `office`, `physicalDeliveryOfficeName`, `l` | `remote` |
| **role** | `role`, `job_title`, `title`, `jobTitle` | `user` |
| **company** | `company` | (omitted from output) |

### User ID Hashing

The `sub` claim is never sent directly. Instead, it is SHA256-hashed and formatted as a UUID-like string for privacy:

```
Input:  "auth0|abc123def456"
Hash:   sha256("auth0|abc123def456") = "a1b2c3d4e5f6..."
Output: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
```

This provides a consistent, irreversible identifier for metric correlation without exposing the raw subject claim.

## Header Mapping

The OTEL Helper outputs a flat JSON object mapping attribute names to HTTP header names:

| User Attribute | HTTP Header | OTEL Collector Resource Attribute |
|---------------|-------------|-----------------------------------|
| email | `x-user-email` | `user.email` |
| user_id | `x-user-id` | `user.id` |
| username | `x-user-name` | `user.name` |
| department | `x-department` | `department` |
| team | `x-team-id` | `team.id` |
| cost_center | `x-cost-center` | `cost_center` |
| organization | `x-organization` | `organization` |
| location | `x-location` | `location` |
| role | `x-role` | `role` |
| manager | `x-manager` | `manager` |
| company | `x-company` | _(not mapped by default)_ |

Headers are lowercase. Only non-empty values are included in the output.

### Example Output

```json
{
  "x-user-email": "alice@acme.com",
  "x-user-id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "x-user-name": "alice",
  "x-department": "engineering",
  "x-team-id": "platform",
  "x-cost-center": "eng-001",
  "x-organization": "okta",
  "x-location": "seattle",
  "x-role": "senior-engineer",
  "x-manager": "bob@acme.com"
}
```

## OIDC Provider Detection

The helper classifies the OIDC provider from the JWT `iss` (issuer) claim by inspecting the hostname:

| Hostname pattern | Provider label |
|-----------------|----------------|
| `*.okta.com` or `okta.com` | `okta` |
| `*.auth0.com` or `auth0.com` | `auth0` |
| `*.microsoftonline.com` or `microsoftonline.com` | `azure` |
| `*.jumpcloud.com` or `jumpcloud.com` | `jc_org` |
| Everything else | `amazon-internal` |

The issuer string is parsed as a URL. If no scheme is present, `https://` is prepended. Hostname matching uses `strings.HasSuffix` with the full domain (e.g., `.okta.com`) to prevent subdomain bypass attacks.

## Infrastructure Integration

### OTEL Collector (ECS Fargate)

Defined in `deployment/infrastructure/otel-collector.yaml`. Key resources:

| Resource | Purpose |
|----------|---------|
| **ECS Cluster** | Fargate cluster with container insights enabled |
| **Task Definition** | ADOT Collector image, 512 CPU / 1024 MB memory |
| **ECS Service** | 1 task, ALB health checks |
| **Application Load Balancer** | Internet-facing, receives OTLP on port 80/443 |
| **Target Group** | Routes to port 4318 on ECS tasks |
| **SSM Parameter** | Stores collector YAML configuration |
| **CloudWatch Log Groups** | `/ecs/otel-collector` (7d retention), `/aws/claude-code/metrics` (30d retention) |

### Collector Pipeline Configuration

```
Receivers                Processors                        Exporters
+---------+     +------------+  +----------+  +-------+     +---------+
| OTLP    |     | attributes |  | resource |  | batch |     | awsemf  |
| gRPC    |---->| (header    |->| (add     |->| (60s  |---->| (CW     |
| HTTP    |     |  mapping)  |  |  acct ID)|  | batch)|     |  EMF)   |
+---------+     +------------+  +----------+  +-------+     +---------+
                                                            +---------+
                                                            | otlp/   |
                                                            | honey-  |
                                                            | comb    |
                                                            | (opt.)  |
                                                            +---------+
```

The attributes processor is what connects the OTEL Helper output to CloudWatch dimensions. It reads the HTTP headers from request metadata and upserts them as span/metric attributes:

```yaml
attributes:
  actions:
    - key: user.email
      from_context: metadata.x-user-email
      action: upsert
    - key: department
      from_context: metadata.x-department
      action: upsert
    # ... 10 total mappings
```

### CloudWatch Metric Dimensions

The `awsemf` exporter declares these dimension rollups:

| Dimensions | Metrics matched |
|-----------|-----------------|
| `[OTelLib]` | All metrics (aggregate) |
| `[department, OTelLib]` | All metrics by department |
| `[team.id, OTelLib]` | All metrics by team |
| `[organization, OTelLib]` | All metrics by org |
| `[model, OTelLib]` | All metrics by model |
| `[cost_center, OTelLib]` | `claude_code.cost.usage`, `claude_code.token.usage` |
| `[type, OTelLib]` | `claude_code.token.usage`, `claude_code.lines_of_code.count` |
| `[tool_name, OTelLib]` | `claude_code.code_edit_tool.*` |
| `[language, OTelLib]` | `claude_code.code_edit_tool.*` |
| `[decision, OTelLib]` | `claude_code.code_edit_tool.decision` |

## Build and Distribution

### Cross-Compilation

The `package` command (`source/claude_code_with_bedrock/cli/commands/package.py`) builds the OTEL Helper only when `profile.monitoring_enabled == True`. It produces platform-specific binaries via Go cross-compilation:

| Platform | Binary name | GOOS/GOARCH |
|----------|-------------|-------------|
| macOS ARM64 | `otel-helper-macos-arm64` | `darwin/arm64` |
| macOS Intel | `otel-helper-macos-intel` | `darwin/amd64` |
| Linux x64 | `otel-helper-linux-x64` | `linux/amd64` |
| Linux ARM64 | `otel-helper-linux-arm64` | `linux/arm64` |
| Windows | `otel-helper-windows.exe` | `windows/amd64` |

### Installation Path

During user installation (`install.sh` / `install.bat`), the binary is copied to:

```
~/claude-code-with-bedrock/otel-helper
```

The `settings.json` references this path:

```json
{
  "otelHeadersHelper": "~/claude-code-with-bedrock/otel-helper"
}
```

## Configuration Reference

### Environment Variables

| Variable | Purpose | Set by |
|----------|---------|--------|
| `CLAUDE_CODE_MONITORING_TOKEN` | JWT token for user attribution (preferred, avoids subprocess call) | Credential Provider |
| `AWS_PROFILE` | Profile name passed to `credential-process --profile` | Claude Code settings.json |
| `DEBUG_MODE` | Enable debug logging (`true`, `1`, `yes`, `y`) | Manual |
| `OTEL_HELPER_LOG_FILE` | File path for log output. When set, debug/info messages write to this file instead of stderr. Warnings and errors always appear on stderr and are additionally written to the file. If the file cannot be opened, falls back to stderr. | Manual |
| `CREDENTIAL_PROCESS_LOG_FILE` | File path for credential process log output. When set, debug messages write to this file instead of stderr. User-facing status/error messages always appear on stderr and are additionally written to the file. If the file cannot be opened, falls back to stderr. | Manual |

### CLI Arguments

```
otel-helper [--test] [--verbose]
```

| Flag | Effect |
|------|--------|
| `--test` | Verbose human-readable output showing all extracted attributes and headers |
| `--verbose` | Enable debug logging to stderr (or to the file specified by `OTEL_HELPER_LOG_FILE` when set) |

Both flags enable debug mode. In normal operation (no flags), only the JSON header object is written to stdout.

### Settings.json Integration

```json
{
  "env": {
    "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
    "OTEL_EXPORTER_OTLP_ENDPOINT": "http://otel-collector-alb-xxxxx.region.elb.amazonaws.com"
  },
  "otelHeadersHelper": "~/claude-code-with-bedrock/otel-helper"
}
```

## Security Considerations

### Privacy

- **User ID hashing**: The `sub` claim is SHA256-hashed before output. The raw subject is never transmitted as a metric attribute.
- **Debug redaction**: When debug mode logs the JWT payload, sensitive fields (`email`, `sub`, `at_hash`, `nonce`) are replaced with `<field-redacted>`.
- **No conversation content**: Only identity metadata is extracted. No message content, prompts, or responses flow through the OTEL Helper.

### Token Handling

- The JWT signature is **not verified** by the OTEL Helper. The token comes from a trusted source (credential-process or the credential provider's environment variable). Signature verification happens at the OIDC provider level during authentication.
- Tokens are never written to disk or logged in normal mode. When `OTEL_HELPER_LOG_FILE` is set with debug mode active, the log file will contain redacted JWT payloads (sensitive fields like `email`, `sub` are replaced with `<field-redacted>`).
- The credential-process call has a 300-second timeout to prevent hangs.

### Failure Behavior

- If no token is available (env var empty, credential-process fails or times out), the helper exits with code 1 and produces no output. Claude Code handles this gracefully - telemetry continues without user attribution headers.
- Malformed JWT tokens result in empty/default attribute values, not crashes. The helper always attempts to produce output with sensible defaults.

### Provider Detection Security

- Issuer URL parsing uses `net/url.Parse` (Go) / `urllib.parse.urlparse` (Python) to prevent hostname bypass attacks.
- Domain matching uses suffix matching with a leading dot (e.g., `.okta.com`), preventing attacks like `evil-okta.com` from being classified as Okta.
