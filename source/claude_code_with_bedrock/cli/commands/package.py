# ABOUTME: Package command for building distribution packages
# ABOUTME: Creates ready-to-distribute packages with embedded configuration

"""Package command - Build distribution packages."""

import json
import os
import platform
import subprocess
from datetime import datetime
from pathlib import Path

import questionary
from cleo.commands.command import Command
from cleo.helpers import option
from rich.console import Console
from rich.panel import Panel
from rich.progress import Progress, SpinnerColumn, TextColumn

from claude_code_with_bedrock.cli.utils.aws import get_stack_outputs
from claude_code_with_bedrock.cli.utils.display import display_configuration_info
from claude_code_with_bedrock.config import Config
from claude_code_with_bedrock.models import (
    get_source_region_for_profile,
)


class PackageCommand(Command):
    """
    Build distribution packages for your organization

    package
        {--target-platform=macos : Target platform (macos, linux, all)}
    """

    name = "package"
    description = "Build distribution packages with embedded configuration"

    options = [
        option(
            "target-platform", description="Target platform for binary (macos, linux, all)", flag=False, default="all"
        ),
        option(
            "profile", description="Configuration profile to use (defaults to active profile)", flag=False, default=None
        ),
        option(
            "status",
            description="[DEPRECATED] Use 'ccwb builds' instead. Check build status by ID or 'latest'",
            flag=False,
            default=None,
        ),
        option("build-verbose", description="Enable verbose logging for build processes", flag=True),
    ]

    def handle(self) -> int:
        """Execute the package command."""
        import platform
        import subprocess

        console = Console()

        # Check if this is a status check (deprecated - moved to builds command)
        if self.option("status") is not None:
            console.print("[yellow]⚠️  DEPRECATED: Status check has moved to the builds command[/yellow]")
            console.print("\nUse one of these commands instead:")
            console.print("  • [cyan]poetry run ccwb builds[/cyan]                    (list all recent builds)")
            console.print("  • [cyan]poetry run ccwb builds --status <build-id>[/cyan] (check specific build)")
            console.print("  • [cyan]poetry run ccwb builds --status latest[/cyan]    (check latest build)")
            console.print("\nRedirecting to builds command...\n")
            return self._check_build_status(self.option("status"), console)

        # Load configuration first (needed to check CodeBuild status)
        config = Config.load()
        # Use specified profile or default to active profile, or fall back to "ClaudeCode"
        profile_name = self.option("profile") or config.active_profile or "ClaudeCode"
        profile = config.get_profile(profile_name)

        if not profile:
            console.print("[red]No deployment found. Run 'poetry run ccwb init' first.[/red]")
            return 1

        # Interactive prompts if not provided via CLI
        target_platform = self.option("target-platform")
        if target_platform == "all":  # Default value, prompt user
            # Build list of available platform choices
            # Go cross-compiles all platforms from any host, so all are always available
            platform_choices = [
                "macos-arm64",
                "macos-intel",
                "linux-x64",
                "linux-arm64",
                "windows",
            ]

            # Use checkbox for multiple selection (require at least one)
            selected_platforms = questionary.checkbox(
                "Which platform(s) do you want to build for? (Use space to select, enter to confirm)",
                choices=platform_choices,
                validate=lambda x: len(x) > 0 or "You must select at least one platform",
            ).ask()

            # Use the selected platforms (guaranteed to have at least one due to validation)
            target_platform = selected_platforms if len(selected_platforms) > 1 else selected_platforms[0]

        # Prompt for co-authorship preference (default to No - opt-in approach)
        include_coauthored_by = questionary.confirm(
            "Include 'Co-Authored-By: Claude' in git commits?",
            default=False,
        ).ask()

        # Validate platform
        valid_platforms = ["macos", "macos-arm64", "macos-intel", "linux", "linux-x64", "linux-arm64", "windows", "all"]
        if isinstance(target_platform, list):
            for platform_name in target_platform:
                if platform_name not in valid_platforms:
                    console.print(
                        f"[red]Invalid platform: {platform_name}. Valid options: {', '.join(valid_platforms)}[/red]"
                    )
                    return 1
        elif target_platform not in valid_platforms:
            console.print(
                f"[red]Invalid platform: {target_platform}. Valid options: {', '.join(valid_platforms)}[/red]"
            )
            return 1

        # Get actual Identity Pool ID or Role ARN from stack outputs
        console.print("[yellow]Fetching deployment information...[/yellow]")
        stack_outputs = get_stack_outputs(
            profile.stack_names.get("auth", f"{profile.identity_pool_name}-stack"), profile.aws_region
        )

        if not stack_outputs:
            console.print("[red]Could not fetch stack outputs. Is the stack deployed?[/red]")
            return 1

        # Check federation type and get appropriate identifier
        federation_type = stack_outputs.get("FederationType", profile.federation_type)
        identity_pool_id = None
        federated_role_arn = None

        if federation_type == "direct":
            # Try DirectSTSRoleArn first (both old and new templates have this for direct mode)
            # Then fallback to FederatedRoleArn (new templates)
            federated_role_arn = stack_outputs.get("DirectSTSRoleArn")
            if not federated_role_arn or federated_role_arn == "N/A":
                federated_role_arn = stack_outputs.get("FederatedRoleArn")
            if not federated_role_arn or federated_role_arn == "N/A":
                console.print("[red]Direct STS Role ARN not found in stack outputs.[/red]")
                return 1
        else:
            identity_pool_id = stack_outputs.get("IdentityPoolId")
            if not identity_pool_id:
                console.print("[red]Identity Pool ID not found in stack outputs.[/red]")
                return 1

        # Welcome
        console.print(
            Panel.fit(
                "[bold cyan]Package Builder[/bold cyan]\n\n"
                f"Creating distribution package for {profile.provider_domain}",
                border_style="cyan",
                padding=(1, 2),
            )
        )

        # Capture git SHA for version tracking in OTEL resource attributes
        git_sha = self._get_git_sha()

        # Create timestamped output directory under profile name
        timestamp = datetime.now().strftime("%Y-%m-%d-%H%M%S")
        output_dir = Path("./dist") / profile_name / timestamp

        # Create output directory
        output_dir.mkdir(parents=True, exist_ok=True)

        # Create embedded configuration based on federation type
        embedded_config = {
            "provider_domain": profile.provider_domain,
            "client_id": profile.client_id,
            "region": profile.aws_region,
            "allowed_bedrock_regions": profile.allowed_bedrock_regions,
            "package_timestamp": timestamp,
            "package_version": f"1.0.0+{git_sha}",
            "federation_type": federation_type,
        }

        # Add federation-specific configuration
        if federation_type == "direct":
            embedded_config["federated_role_arn"] = federated_role_arn
            embedded_config["max_session_duration"] = profile.max_session_duration
        else:
            embedded_config["identity_pool_id"] = identity_pool_id

        # Show what will be packaged using shared display utility
        display_configuration_info(profile, identity_pool_id or federated_role_arn, format_type="simple")

        # Build package
        console.print("\n[bold]Building package...[/bold]")

        # Build executable(s) using Go cross-compilation
        # Go can cross-compile all platforms from any host, so no Docker/Rosetta/CodeBuild needed
        if isinstance(target_platform, list):
            platforms_to_build = []
            for platform_choice in target_platform:
                if platform_choice == "all":
                    platforms_to_build.extend(["macos-arm64", "macos-intel", "linux-x64", "linux-arm64", "windows"])
                elif platform_choice not in platforms_to_build:
                    platforms_to_build.append(platform_choice)
        elif target_platform == "all":
            platforms_to_build = ["macos-arm64", "macos-intel", "linux-x64", "linux-arm64", "windows"]
        else:
            platforms_to_build = [target_platform]

        built_executables = []
        built_otel_helpers = []

        console.print()
        for platform_name in platforms_to_build:
            # Build credential process (Go cross-compilation - always local)
            console.print(f"[cyan]Building credential process for {platform_name}...[/cyan]")
            try:
                executable_path = self._build_executable(output_dir, platform_name)
                built_executables.append((platform_name, executable_path))
            except Exception as e:
                console.print(f"[yellow]Warning: Could not build credential process for {platform_name}: {e}[/yellow]")

            # Build OTEL helper if monitoring is enabled
            if profile.monitoring_enabled:
                console.print(f"[cyan]Building OTEL helper for {platform_name}...[/cyan]")
                try:
                    otel_helper_path = self._build_otel_helper(output_dir, platform_name)
                    if otel_helper_path is not None:
                        built_otel_helpers.append((platform_name, otel_helper_path))
                except Exception as e:
                    console.print(f"[yellow]Warning: Could not build OTEL helper for {platform_name}: {e}[/yellow]")

        # Check if any binaries were built
        if not built_executables:
            console.print("\n[red]Error: No binaries were successfully built.[/red]")
            console.print("Please check the error messages above.")
            return 1

        # Create configuration
        console.print("\n[cyan]Creating configuration...[/cyan]")
        # Pass the appropriate identifier based on federation type
        federation_identifier = federated_role_arn if federation_type == "direct" else identity_pool_id
        self._create_config(output_dir, profile, federation_identifier, federation_type, profile_name)

        # Create installer
        console.print("[cyan]Creating installer script...[/cyan]")
        self._create_installer(output_dir, profile, built_executables, built_otel_helpers)

        # Create documentation
        console.print("[cyan]Creating documentation...[/cyan]")
        self._create_documentation(output_dir, profile, timestamp)

        # Always create Claude Code settings (required for Bedrock configuration)
        console.print("[cyan]Creating Claude Code settings...[/cyan]")
        self._create_claude_settings(output_dir, profile, include_coauthored_by, profile_name, git_sha)

        # Summary
        console.print("\n[green]✓ Package created successfully![/green]")
        console.print(f"\nOutput directory: [cyan]{output_dir}[/cyan]")
        console.print("\nPackage contents:")

        # Show which binaries were built
        for platform_name, executable_path in built_executables:
            binary_name = executable_path.name
            console.print(f"  • {binary_name} - Authentication executable for {platform_name}")

        console.print("  • config.json - Configuration")
        console.print("  • install.sh - Installation script for macOS/Linux")
        # Check if Windows installer exists (created when Windows binaries are present)
        if (output_dir / "install.bat").exists():
            console.print("  • install.bat - Installation script for Windows")
        console.print("  • README.md - Installation instructions")
        if profile.monitoring_enabled and (output_dir / "claude-settings" / "settings.json").exists():
            console.print("  • claude-settings/settings.json - Claude Code telemetry settings")
            for platform_name, otel_helper_path in built_otel_helpers:
                console.print(f"  • {otel_helper_path.name} - OTEL helper executable for {platform_name}")

        # Next steps
        console.print("\n[bold]Distribution steps:[/bold]")
        console.print("1. Send users the entire dist folder")
        console.print("2. Users run: ./install.sh")
        console.print("3. Authentication is configured automatically")

        console.print("\n[bold]To test locally:[/bold]")
        console.print(f"cd {output_dir}")
        console.print("./install.sh")

        # Show next steps
        console.print("\n[bold]Next steps:[/bold]")

        # Only show distribute command if distribution is enabled
        if profile.enable_distribution:
            console.print("To create a distribution package: [cyan]poetry run ccwb distribute[/cyan]")
        else:
            console.print("Share the dist folder with your users for installation")

        return 0

    def _check_build_status(self, build_id: str, console: Console) -> int:
        """Check the status of a CodeBuild build."""
        import json
        from pathlib import Path

        import boto3

        try:
            # If no build ID provided, check for latest
            if not build_id or build_id == "latest":
                build_info_file = Path.home() / ".claude-code" / "latest-build.json"
                if not build_info_file.exists():
                    console.print("[red]No recent builds found. Start a build with 'poetry run ccwb package'[/red]")
                    return 1

                with open(build_info_file) as f:
                    build_info = json.load(f)
                    build_id = build_info["build_id"]
                    console.print(f"[dim]Checking latest build: {build_id}[/dim]")

            # Get build status from CodeBuild
            # Load profile to get the correct region
            config = Config.load()
            profile_name = self.option("profile")
            profile = config.get_profile(profile_name)
            if not profile:
                console.print("[red]No configuration found. Run 'poetry run ccwb init' first.[/red]")
                return 1

            codebuild = boto3.client("codebuild", region_name=profile.aws_region)
            response = codebuild.batch_get_builds(ids=[build_id])

            if not response.get("builds"):
                console.print(f"[red]Build not found: {build_id}[/red]")
                return 1

            build = response["builds"][0]
            status = build["buildStatus"]

            # Display status
            if status == "IN_PROGRESS":
                console.print("[yellow]⏳ Build in progress[/yellow]")
                console.print(f"Phase: {build.get('currentPhase', 'Unknown')}")
                if "startTime" in build:
                    from datetime import datetime

                    start_time = build["startTime"]
                    elapsed = datetime.now(start_time.tzinfo) - start_time
                    console.print(f"Elapsed: {int(elapsed.total_seconds() / 60)} minutes")
            elif status == "SUCCEEDED":
                console.print("[green]✓ Build succeeded![/green]")
                console.print(f"Duration: {build.get('buildDurationInMinutes', 'Unknown')} minutes")
                console.print("\n[bold]Windows build artifacts are ready![/bold]")
                console.print("Next steps:")
                console.print("  Run: [cyan]poetry run ccwb distribute[/cyan]")
                console.print("  This will download Windows artifacts from S3 and create your distribution package")
            else:
                console.print(f"[red]✗ Build {status.lower()}[/red]")
                if "phases" in build:
                    for phase in build["phases"]:
                        if phase.get("phaseStatus") == "FAILED":
                            console.print(f"[red]Failed in phase: {phase.get('phaseType')}[/red]")

            # Show console link
            project_name = build_id.split(":")[0]
            build_uuid = build_id.split(":")[1]
            console.print(
                f"\n[dim]View logs: https://console.aws.amazon.com/codesuite/codebuild/projects/{project_name}/build/{build_uuid}[/dim]"
            )

            return 0

        except Exception as e:
            console.print(f"[red]Error checking build status: {e}[/red]")
            return 1

    # Map from platform selection names to (GOOS, GOARCH, binary_name)
    GO_PLATFORM_MAP = {
        "macos-arm64": ("darwin", "arm64", "credential-process-macos-arm64"),
        "macos-intel": ("darwin", "amd64", "credential-process-macos-intel"),
        "linux-x64": ("linux", "amd64", "credential-process-linux-x64"),
        "linux-arm64": ("linux", "arm64", "credential-process-linux-arm64"),
        "windows": ("windows", "amd64", "credential-process-windows.exe"),
    }

    GO_OTEL_PLATFORM_MAP = {
        "macos-arm64": ("darwin", "arm64", "otel-helper-macos-arm64"),
        "macos-intel": ("darwin", "amd64", "otel-helper-macos-intel"),
        "linux-x64": ("linux", "amd64", "otel-helper-linux-x64"),
        "linux-arm64": ("linux", "arm64", "otel-helper-linux-arm64"),
        "windows": ("windows", "amd64", "otel-helper-windows.exe"),
    }

    def _build_executable(self, output_dir: Path, target_platform: str) -> Path:
        """Build credential-process binary for target platform using Go cross-compilation.

        Go cross-compiles all platforms from any host with CGO_ENABLED=0,
        so no Docker, Rosetta, CodeBuild, or platform-specific tooling is needed.
        """
        import platform as platform_module

        # Handle "macos" and "linux" aliases
        if target_platform == "macos":
            current_machine = platform_module.machine().lower()
            target_platform = "macos-arm64" if current_machine == "arm64" else "macos-intel"
        elif target_platform == "linux":
            current_machine = platform_module.machine().lower()
            target_platform = "linux-arm64" if current_machine in ("arm64", "aarch64") else "linux-x64"

        if target_platform not in self.GO_PLATFORM_MAP:
            raise ValueError(f"Unsupported target platform: {target_platform}")

        goos, goarch, binary_name = self.GO_PLATFORM_MAP[target_platform]
        return self._build_go_binary_from(output_dir, goos, goarch, binary_name, "credential-provider-go")

    def _build_go_binary_from(self, output_dir: Path, goos: str, goarch: str, binary_name: str, go_subdir: str) -> Path:
        """Cross-compile a Go binary for the given GOOS/GOARCH from the specified source subdirectory."""
        console = Console()
        verbose = self.option("build-verbose")

        # Locate the Go source directory
        source_dir = Path(__file__).parent.parent.parent.parent
        go_src_dir = source_dir / go_subdir

        if not (go_src_dir / "go.mod").exists():
            raise FileNotFoundError(
                f"Go source not found at {go_src_dir}. "
                f"Ensure {go_subdir}/ exists with go.mod."
            )

        # Verify Go is installed
        go_check = subprocess.run(["go", "version"], capture_output=True, text=True)
        if go_check.returncode != 0:
            raise RuntimeError(
                "Go compiler not found. Install from https://go.dev/dl/\n"
                "Go is required to build credential-process binaries."
            )

        console.print(f"[yellow]Building {binary_name} (Go {goos}/{goarch})...[/yellow]")

        env = os.environ.copy()
        env["CGO_ENABLED"] = "0"
        env["GOOS"] = goos
        env["GOARCH"] = goarch

        cmd = [
            "go", "build",
            "-ldflags", "-s -w",
            "-o", str((output_dir / binary_name).resolve()),
            ".",
        ]

        result = subprocess.run(
            cmd,
            cwd=go_src_dir,
            env=env,
            capture_output=not verbose,
            text=True,
        )

        if result.returncode != 0:
            error_detail = result.stderr if result.stderr else "Unknown error"
            raise RuntimeError(f"Go build failed for {goos}/{goarch}: {error_detail}")

        binary_path = output_dir / binary_name
        if not binary_path.exists():
            raise RuntimeError(f"Binary not created: {binary_path}")

        # Set executable permission (no-op on Windows targets, but harmless)
        binary_path.chmod(0o755)

        size_mb = binary_path.stat().st_size / (1024 * 1024)
        console.print(f"[green]✓ {binary_name} built ({size_mb:.1f} MB)[/green]")
        return binary_path

    def _build_otel_helper(self, output_dir: Path, target_platform: str) -> Path:
        """Build OTEL helper binary for target platform using Go cross-compilation."""
        import platform as platform_module

        # Handle "macos" and "linux" aliases
        if target_platform == "macos":
            current_machine = platform_module.machine().lower()
            target_platform = "macos-arm64" if current_machine == "arm64" else "macos-intel"
        elif target_platform == "linux":
            current_machine = platform_module.machine().lower()
            target_platform = "linux-arm64" if current_machine in ("arm64", "aarch64") else "linux-x64"

        if target_platform not in self.GO_OTEL_PLATFORM_MAP:
            raise ValueError(f"Unsupported target platform for OTEL helper: {target_platform}")

        goos, goarch, binary_name = self.GO_OTEL_PLATFORM_MAP[target_platform]
        return self._build_go_binary_from(output_dir, goos, goarch, binary_name, "otel-helper-go")

    def _get_git_sha(self) -> str:
        """Get short git SHA of current HEAD, or 'unknown' if not in a git repo."""
        try:
            result = subprocess.run(
                ["git", "rev-parse", "--short", "HEAD"],
                capture_output=True, text=True, timeout=5,
            )
            if result.returncode == 0:
                return result.stdout.strip()
        except Exception:
            pass
        return "unknown"

    def _create_config(
        self,
        output_dir: Path,
        profile,
        federation_identifier: str,
        federation_type: str = "cognito",
        profile_name: str = "ClaudeCode",
    ) -> Path:
        """Create the configuration file.

        Args:
            output_dir: Directory to write config.json to
            profile: Profile object with configuration
            federation_identifier: Identity pool ID or role ARN
            federation_type: "cognito" or "direct"
            profile_name: Name to use as key in config.json (defaults to "ClaudeCode" for backward compatibility)
        """
        config = {
            profile_name: {
                "provider_domain": profile.provider_domain,
                "client_id": profile.client_id,
                "aws_region": profile.aws_region,
                "provider_type": profile.provider_type or self._detect_provider_type(profile.provider_domain),
                "credential_storage": profile.credential_storage,
                "cross_region_profile": profile.cross_region_profile or "us",
            }
        }

        # Add the appropriate federation field based on type
        if federation_type == "direct":
            config[profile_name]["federated_role_arn"] = federation_identifier
            config[profile_name]["federation_type"] = "direct"
            config[profile_name]["max_session_duration"] = profile.max_session_duration
        else:
            config[profile_name]["identity_pool_id"] = federation_identifier
            config[profile_name]["federation_type"] = "cognito"

        # Add cognito_user_pool_id if it's a Cognito provider
        if profile.provider_type == "cognito" and profile.cognito_user_pool_id:
            config[profile_name]["cognito_user_pool_id"] = profile.cognito_user_pool_id

        # Add selected_model if available
        if hasattr(profile, "selected_model") and profile.selected_model:
            config[profile_name]["selected_model"] = profile.selected_model

        config_path = output_dir / "config.json"
        with open(config_path, "w") as f:
            json.dump(config, f, indent=2)
        return config_path

    def _get_bedrock_region_for_profile(self, profile) -> str:
        """Get the correct AWS region for Bedrock API calls based on user-selected source region."""
        return get_source_region_for_profile(profile)

    def _detect_provider_type(self, domain: str) -> str:
        """Auto-detect provider type from domain."""
        from urllib.parse import urlparse

        if not domain:
            return "oidc"

        # Handle both full URLs and domain-only inputs
        url_to_parse = domain if domain.startswith(("http://", "https://")) else f"https://{domain}"

        try:
            parsed = urlparse(url_to_parse)
            hostname = parsed.hostname

            if not hostname:
                return "oidc"

            hostname_lower = hostname.lower()

            # Check for exact domain match or subdomain match
            # Using endswith with leading dot prevents bypass attacks
            if hostname_lower.endswith(".okta.com") or hostname_lower == "okta.com":
                return "okta"
            elif hostname_lower.endswith(".auth0.com") or hostname_lower == "auth0.com":
                return "auth0"
            elif hostname_lower.endswith(".microsoftonline.com") or hostname_lower == "microsoftonline.com":
                return "azure"
            elif hostname_lower.endswith(".windows.net") or hostname_lower == "windows.net":
                return "azure"
            elif hostname_lower.endswith(".amazoncognito.com") or hostname_lower == "amazoncognito.com":
                return "cognito"
            else:
                return "oidc"  # Default to generic OIDC
        except Exception:
            return "oidc"  # Default to generic OIDC on parsing error

    def _create_installer(self, output_dir: Path, profile, built_executables, built_otel_helpers=None) -> Path:
        """Create simple installer script."""

        # Determine which binaries were built
        platforms_built = [platform for platform, _ in built_executables]
        [platform for platform, _ in built_otel_helpers] if built_otel_helpers else []

        installer_content = f"""#!/bin/bash
# Claude Code Authentication Installer
# Organization: {profile.provider_domain}
# Generated: {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}

set -e

echo "======================================"
echo "Claude Code Authentication Installer"
echo "======================================"
echo
echo "Organization: {profile.provider_domain}"
echo


# Check prerequisites
echo "Checking prerequisites..."

if ! command -v aws &> /dev/null; then
    echo "❌ AWS CLI is not installed"
    echo "   Please install from https://aws.amazon.com/cli/"
    exit 1
fi

echo "✓ Prerequisites found"

# Detect platform and architecture
echo
echo "Detecting platform and architecture..."
if [[ "$OSTYPE" == "darwin"* ]]; then
    PLATFORM="macos"
    ARCH=$(uname -m)
    if [[ "$ARCH" == "arm64" ]]; then
        echo "✓ Detected macOS ARM64 (Apple Silicon)"
        BINARY_SUFFIX="macos-arm64"
    else
        echo "✓ Detected macOS Intel"
        BINARY_SUFFIX="macos-intel"
    fi
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    PLATFORM="linux"
    ARCH=$(uname -m)
    if [[ "$ARCH" == "aarch64" ]] || [[ "$ARCH" == "arm64" ]]; then
        echo "✓ Detected Linux ARM64"
        BINARY_SUFFIX="linux-arm64"
    else
        echo "✓ Detected Linux x64"
        BINARY_SUFFIX="linux-x64"
    fi
else
    echo "❌ Unsupported platform: $OSTYPE"
    echo "   This installer supports macOS and Linux only."
    exit 1
fi

# Check if binary for platform exists
CREDENTIAL_BINARY="credential-process-$BINARY_SUFFIX"
OTEL_BINARY="otel-helper-$BINARY_SUFFIX"

if [ ! -f "$CREDENTIAL_BINARY" ]; then
    echo "❌ Binary not found for your platform: $CREDENTIAL_BINARY"
    echo "   Please ensure you have the correct package for your architecture."
    exit 1
fi
"""

        installer_content += f"""
# Create directory
echo
echo "Installing authentication tools..."
mkdir -p ~/claude-code-with-bedrock

# Copy appropriate binary
cp "$CREDENTIAL_BINARY" ~/claude-code-with-bedrock/credential-process

# Copy config
cp config.json ~/claude-code-with-bedrock/
chmod +x ~/claude-code-with-bedrock/credential-process

# macOS Keychain Notice
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo
    echo "⚠️  macOS Keychain Access:"
    echo "   On first use, macOS will ask for permission to access the keychain."
    echo "   This is normal and required for secure credential storage."
    echo "   Click 'Always Allow' when prompted."
fi

# Copy Claude Code settings if present
if [ -d "claude-settings" ]; then
    echo
    echo "Installing Claude Code settings..."
    mkdir -p ~/.claude

    # Copy settings and replace placeholders
    if [ -f "claude-settings/settings.json" ]; then
        # Check if settings file already exists
        if [ -f ~/.claude/settings.json ]; then
            echo "Existing Claude Code settings found"
            read -p "Overwrite with new settings? (Y/n): " -n 1 -r
            echo
            # Default to Yes if user just presses enter (empty REPLY)
            if [[ -z "$REPLY" ]]; then
                REPLY="y"
            fi
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                echo "Skipping Claude Code settings..."
                SKIP_SETTINGS=true
            fi
        fi

        if [ "$SKIP_SETTINGS" != "true" ]; then
            # Replace placeholders and write settings
            sed -e "s|__OTEL_HELPER_PATH__|$HOME/claude-code-with-bedrock/otel-helper|g" \
                -e "s|__CREDENTIAL_PROCESS_PATH__|$HOME/claude-code-with-bedrock/credential-process|g" \
                "claude-settings/settings.json" > ~/.claude/settings.json
            echo "✓ Claude Code settings configured"
        fi
    fi
fi

# Copy OTEL helper executable if present
if [ -f "$OTEL_BINARY" ]; then
    echo
    echo "Installing OTEL helper..."
    cp "$OTEL_BINARY" ~/claude-code-with-bedrock/otel-helper
    chmod +x ~/claude-code-with-bedrock/otel-helper
    echo "✓ OTEL helper installed"
fi

# Add debug info if OTEL helper was installed
if [ -f ~/claude-code-with-bedrock/otel-helper ]; then
    echo "The OTEL helper will extract user attributes from authentication tokens"
    echo "and include them in metrics. To test the helper, run:"
    echo "  ~/claude-code-with-bedrock/otel-helper --test"
fi

# Update AWS config
echo
echo "Configuring AWS profiles..."
mkdir -p ~/.aws

# Read all profiles from config.json
PROFILES=$(python3 -c "import json; profiles = list(json.load(open('config.json')).keys()); print(' '.join(profiles))")

if [ -z "$PROFILES" ]; then
    echo "❌ No profiles found in config.json"
    exit 1
fi

echo "Found profiles: $PROFILES"
echo

# Get region from package settings (for Bedrock calls, not infrastructure)
if [ -f "claude-settings/settings.json" ]; then
    DEFAULT_REGION=$(python3 -c "import json; print(json.load(open('claude-settings/settings.json'))[
    'env']['AWS_REGION'])" 2>/dev/null || echo "{profile.aws_region}")
else
    DEFAULT_REGION="{profile.aws_region}"
fi

# Configure each profile
for PROFILE_NAME in $PROFILES; do
    echo "Configuring AWS profile: $PROFILE_NAME"

    # Remove old profile if exists
    sed -i.bak "/\\[profile $PROFILE_NAME\\]/,/^$/d" ~/.aws/config 2>/dev/null || true

    # Get profile-specific region from config.json
    PROFILE_REGION=$(python3 -c "import json; print(json.load(open('config.json')).get('$PROFILE_NAME', \
    {{}}).get('aws_region', '$DEFAULT_REGION'))")

    # Add new profile with --profile flag (cross-platform, no shell required)
    cat >> ~/.aws/config << EOF
[profile $PROFILE_NAME]
credential_process = $HOME/claude-code-with-bedrock/credential-process --profile $PROFILE_NAME
region = $PROFILE_REGION
EOF
    echo "  ✓ Created AWS profile '$PROFILE_NAME'"
done

echo
echo "======================================"
echo "✓ Installation complete!"
echo "======================================"
echo
echo "Available profiles:"
for PROFILE_NAME in $PROFILES; do
    echo "  - $PROFILE_NAME"
done
echo
echo "To use Claude Code authentication:"
echo "  export AWS_PROFILE=<profile-name>"
echo "  aws sts get-caller-identity"
echo
echo "Example:"
FIRST_PROFILE=$(echo $PROFILES | awk '{{print $1}}')
echo "  export AWS_PROFILE=$FIRST_PROFILE"
echo "  aws sts get-caller-identity"
echo
echo "Note: Authentication will automatically open your browser when needed."
echo
"""

        installer_path = output_dir / "install.sh"
        with open(installer_path, "w") as f:
            f.write(installer_content)
        installer_path.chmod(0o755)

        # Create Windows installer only if Windows builds are enabled (CodeBuild)
        if "windows" in platforms_built or (hasattr(profile, "enable_codebuild") and profile.enable_codebuild):
            self._create_windows_installer(output_dir, profile)

        return installer_path

    def _create_windows_installer(self, output_dir: Path, profile) -> Path:
        """Create Windows batch installer script."""

        installer_content = f"""@echo off
REM Claude Code Authentication Installer for Windows
REM Organization: {profile.provider_domain}
REM Generated: {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}

echo ======================================
echo Claude Code Authentication Installer
echo ======================================
echo.
echo Organization: {profile.provider_domain}
echo.

REM Check prerequisites
echo Checking prerequisites...

where aws >nul 2>&1
if %errorlevel% neq 0 (
    echo ERROR: AWS CLI is not installed
    echo        Please install from https://aws.amazon.com/cli/
    pause
    exit /b 1
)

echo OK Prerequisites found
echo.

REM Create directory
echo Installing authentication tools...
if not exist "%USERPROFILE%\\claude-code-with-bedrock" mkdir "%USERPROFILE%\\claude-code-with-bedrock"

REM Copy credential process executable with renamed target
echo Copying credential process...
copy /Y "credential-process-windows.exe" "%USERPROFILE%\\claude-code-with-bedrock\\credential-process.exe" >nul
if %errorlevel% neq 0 (
    echo ERROR: Failed to copy credential-process-windows.exe
    pause
    exit /b 1
)

REM Copy OTEL helper if it exists with renamed target
if exist "otel-helper-windows.exe" (
    echo Copying OTEL helper...
    copy /Y "otel-helper-windows.exe" "%USERPROFILE%\\claude-code-with-bedrock\\otel-helper.exe" >nul
)

REM Copy configuration
echo Copying configuration...
copy /Y "config.json" "%USERPROFILE%\\claude-code-with-bedrock\\" >nul

REM Copy Claude Code settings if they exist
if exist "claude-settings" (
    echo Copying Claude Code telemetry settings...
    if not exist "%USERPROFILE%\\.claude" mkdir "%USERPROFILE%\\.claude"

    REM Copy settings and replace placeholders
    if exist "claude-settings\\settings.json" (
        set SKIP_SETTINGS=false
        if exist "%USERPROFILE%\\.claude\\settings.json" (
            echo Existing Claude Code settings found
            set /p OVERWRITE="Overwrite with new settings? (y/n): "
            if /i not "%OVERWRITE%"=="y" (
                echo Skipping Claude Code settings...
                set SKIP_SETTINGS=true
            )
        )

        if not "%SKIP_SETTINGS%"=="true" (
            REM Use PowerShell to replace placeholders
            powershell -Command ^
            "$otelPath = '%USERPROFILE%\\\\claude-code-with-bedrock\\\\otel-helper.exe' ^
            -replace '\\\\\\\\', '/'; ^
            $credPath = '%USERPROFILE%\\\\claude-code-with-bedrock\\\\credential-process.exe' ^
            -replace '\\\\\\\\', '/'; ^
            (Get-Content 'claude-settings\\\\settings.json') ^
            -replace '__OTEL_HELPER_PATH__', $otelPath ^
            -replace '__CREDENTIAL_PROCESS_PATH__', $credPath | ^
            Set-Content '%USERPROFILE%\\\\.claude\\\\settings.json'"
            echo OK Claude Code settings configured
        )
    )
)

REM Configure AWS profiles
echo.
echo Configuring AWS profiles...

REM Read profiles from config.json using PowerShell
for /f %%p in ('powershell -Command ^
"& {{$c=Get-Content config.json|ConvertFrom-Json;$c.PSObject.Properties.Name}}"') do (
    echo Configuring AWS profile: %%p

    REM Get profile-specific region
    for /f %%r in ('powershell -Command ^
    "& {{$c=Get-Content config.json|ConvertFrom-Json;$c.'%%p'.aws_region}}"') do set PROFILE_REGION=%%r


    REM Set credential process with --profile flag (cross-platform, no wrapper needed)
    aws configure set credential_process ^
    "%USERPROFILE%\\claude-code-with-bedrock\\credential-process.exe --profile %%p" --profile %%p


    REM Set region
    if defined PROFILE_REGION (
        aws configure set region !PROFILE_REGION! --profile %%p
    ) else (
        aws configure set region {profile.aws_region} --profile %%p
    )

    echo   OK Created AWS profile '%%p'
)

echo.
echo ======================================
echo Installation complete!
echo ======================================
echo.
echo Available profiles:
for /f %%p in ('powershell -Command ^
"$config = Get-Content config.json | ConvertFrom-Json; $config.PSObject.Properties.Name"') do (
    echo   - %%p
)
echo.
echo To use Claude Code authentication:
echo   set AWS_PROFILE=^<profile-name^>
echo   aws sts get-caller-identity
echo.
echo Example:
for /f %%p in ('powershell -Command ^
"$config = Get-Content config.json | ConvertFrom-Json; $config.PSObject.Properties.Name | Select-Object -First 1"') do (
    echo   set AWS_PROFILE=%%p
    echo   aws sts get-caller-identity
)
echo.
echo Note: Authentication will automatically open your browser when needed.
echo.
pause
"""

        installer_path = output_dir / "install.bat"
        with open(installer_path, "w", encoding="utf-8") as f:
            f.write(installer_content)

        # Note: chmod not needed on Windows batch files
        return installer_path

    def _create_documentation(self, output_dir: Path, profile, timestamp: str):
        """Create user documentation."""
        readme_content = f"""# Claude Code Authentication Setup

## Quick Start

### macOS/Linux

1. Extract the package:
   ```bash
   unzip claude-code-package-*.zip
   cd claude-code-package
   ```

2. Run the installer:
   ```bash
   ./install.sh
   ```

3. Use the AWS profile:
   ```bash
   export AWS_PROFILE=ClaudeCode
   aws sts get-caller-identity
   ```

### Windows

#### Step 1: Download the Package
```powershell
# Use the Invoke-WebRequest command provided by your IT administrator
Invoke-WebRequest -Uri "URL_PROVIDED" -OutFile "claude-code-package.zip"
```

#### Step 2: Extract the Package

**Option A: Using Windows Explorer**
1. Right-click on `claude-code-package.zip`
2. Select "Extract All..."
3. Choose a destination folder
4. Click "Extract"

**Option B: Using PowerShell**
```powershell
# Extract to current directory
Expand-Archive -Path "claude-code-package.zip" -DestinationPath "claude-code-package"

# Navigate to the extracted folder
cd claude-code-package
```

**Option C: Using Command Prompt**
```cmd
# If you have tar available (Windows 10 1803+)
tar -xf claude-code-package.zip

# Or use PowerShell from Command Prompt
powershell -command "Expand-Archive -Path 'claude-code-package.zip' -DestinationPath 'claude-code-package'"

cd claude-code-package
```

#### Step 3: Run the Installer
```cmd
install.bat
```

The installer will:
- Check for AWS CLI installation
- Copy authentication tools to `%USERPROFILE%\\claude-code-with-bedrock`
- Configure the AWS profile "ClaudeCode"
- Test the authentication

#### Step 4: Use Claude Code
```cmd
# Set the AWS profile
set AWS_PROFILE=ClaudeCode

# Verify authentication works
aws sts get-caller-identity

# Your browser will open automatically for authentication if needed
```

For PowerShell users:
```powershell
$env:AWS_PROFILE = "ClaudeCode"
aws sts get-caller-identity
```

## What This Does

- Installs the Claude Code authentication tools
- Configures your AWS CLI to use {profile.provider_domain} for authentication
- Sets up automatic credential refresh via your browser

## Requirements

- Python 3.8 or later
- AWS CLI v2
- pip3

## Troubleshooting

### macOS Keychain Access Popup
On first use, macOS will ask for permission to access the keychain. This is normal and required for \
secure credential storage. Click "Always Allow" to avoid repeated prompts.

### Authentication Issues
If you encounter issues with authentication:
- Ensure you're assigned to the Claude Code application in your identity provider
- Check that port 8400 is available for the callback
- Contact your IT administrator for help

### Authentication Behavior

The system handles authentication automatically:
- Your browser will open when authentication is needed
- Credentials are cached securely to avoid repeated logins
- Bad credentials are automatically cleared and re-authenticated

To manually clear cached credentials (if needed):
```bash
~/claude-code-with-bedrock/credential-process --clear-cache
```

This will force re-authentication on your next AWS command.

### Browser doesn't open
Check that you're not in an SSH session. The browser needs to open on your local machine.

## Support

Contact your IT administrator for help.

Configuration Details:
- Organization: {profile.provider_domain}
- Region: {profile.aws_region}
- Package Version: {timestamp}"""

        # Add analytics information if enabled
        if profile.monitoring_enabled and getattr(profile, "analytics_enabled", True):
            analytics_section = f"""

## Analytics Dashboard

Your organization has enabled advanced analytics for Claude Code usage. You can access detailed metrics \
and reports through AWS Athena.

To view analytics:
1. Open the AWS Console in region {profile.aws_region}
2. Navigate to Athena
3. Select the analytics workgroup and database
4. Run pre-built queries or create custom reports

Available metrics include:
- Token usage by user
- Cost allocation
- Model usage patterns
- Activity trends
"""
            readme_content += analytics_section

        readme_content += "\n" ""

        with open(output_dir / "README.md", "w") as f:
            f.write(readme_content)

    def _create_claude_settings(
        self, output_dir: Path, profile, include_coauthored_by: bool = True, profile_name: str = "ClaudeCode",
        git_sha: str = "unknown",
    ):
        """Create Claude Code settings.json with Bedrock and optional monitoring configuration."""
        console = Console()

        try:
            # Create claude-settings directory (visible, not hidden)
            claude_dir = output_dir / "claude-settings"
            claude_dir.mkdir(exist_ok=True)

            # Start with basic settings required for Bedrock
            settings = {
                "env": {
                    # Set AWS_REGION based on cross-region profile for correct Bedrock endpoint
                    "AWS_REGION": self._get_bedrock_region_for_profile(profile),
                    "CLAUDE_CODE_USE_BEDROCK": "1",
                    # AWS_PROFILE is used by both AWS SDK and otel-helper
                    "AWS_PROFILE": profile_name,
                }
            }

            # Add includeCoAuthoredBy setting if user wants to disable it (Claude Code defaults to true)
            # Only add the field if the user wants it disabled
            if not include_coauthored_by:
                settings["includeCoAuthoredBy"] = False

            # Add awsAuthRefresh for session-based credential storage
            if profile.credential_storage == "session":
                settings["awsAuthRefresh"] = f"__CREDENTIAL_PROCESS_PATH__ --profile {profile_name}"

            # Add selected model as environment variable if available
            if hasattr(profile, "selected_model") and profile.selected_model:
                settings["env"]["ANTHROPIC_MODEL"] = profile.selected_model

                # Determine and set small/fast model based on selected model family
                if "opus" in profile.selected_model:
                    # For Opus, use Haiku as small/fast model
                    model_id = profile.selected_model
                    prefix = model_id.split(".anthropic")[0]  # Get us/eu/apac prefix
                    settings["env"]["ANTHROPIC_SMALL_FAST_MODEL"] = f"{prefix}.anthropic.claude-haiku-4-5-20251001-v1:0"
                else:
                    # For other models, use same model as small/fast (or could use Haiku)
                    settings["env"]["ANTHROPIC_SMALL_FAST_MODEL"] = profile.selected_model

            # If monitoring is enabled, add telemetry configuration
            if profile.monitoring_enabled:
                # Get monitoring stack outputs
                monitoring_stack = profile.stack_names.get("monitoring", f"{profile.identity_pool_name}-otel-collector")
                cmd = [
                    "aws",
                    "cloudformation",
                    "describe-stacks",
                    "--stack-name",
                    monitoring_stack,
                    "--region",
                    profile.aws_region,
                    "--query",
                    "Stacks[0].Outputs",
                    "--output",
                    "json",
                ]

                result = subprocess.run(cmd, capture_output=True, text=True)
                if result.returncode == 0:
                    outputs = json.loads(result.stdout)
                    endpoint = None

                    for output in outputs:
                        if output["OutputKey"] == "CollectorEndpoint":
                            endpoint = output["OutputValue"]
                            break

                    if endpoint:
                        # Add monitoring configuration
                        settings["env"].update(
                            {
                                "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
                                "OTEL_METRICS_EXPORTER": "otlp",
                                "OTEL_LOGS_EXPORTER": "otlp",
                                "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
                                "OTEL_EXPORTER_OTLP_ENDPOINT": endpoint,
                                "OTEL_LOG_TOOL_DETAILS": "1",
                                "OTEL_METRICS_INCLUDE_VERSION": "true",
                                # Add basic OTEL resource attributes for multi-team support
                                # Note: department is NOT included here — the otel-helper extracts
                                # per-user department from the JWT token via the x-department header
                                "OTEL_RESOURCE_ATTRIBUTES": f"team.id=default,"
                                f"cost_center=default,organization=default,"
                                f"ccwb.version={git_sha}",
                            }
                        )

                        # Add the helper executable for generating OTEL headers with user attributes
                        # Use a placeholder that will be replaced by the installer script based on platform
                        settings["otelHeadersHelper"] = "__OTEL_HELPER_PATH__"

                        is_https = endpoint.startswith("https://")
                        console.print(f"[dim]Added monitoring with {'HTTPS' if is_https else 'HTTP'} endpoint[/dim]")
                        if not is_https:
                            console.print(
                                "[dim]WARNING: Using HTTP endpoint - consider enabling HTTPS for production[/dim]"
                            )
                    else:
                        console.print("[yellow]Warning: No monitoring endpoint found in stack outputs[/yellow]")
                else:
                    console.print("[yellow]Warning: Could not fetch monitoring stack outputs[/yellow]")

            # Save settings.json
            settings_path = claude_dir / "settings.json"
            with open(settings_path, "w") as f:
                json.dump(settings, f, indent=2)

            console.print("[dim]Created Claude Code settings for Bedrock configuration[/dim]")

        except Exception as e:
            console.print(f"[yellow]Warning: Could not create Claude Code settings: {e}[/yellow]")
