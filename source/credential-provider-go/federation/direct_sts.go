package federation

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"credential-provider-go/config"
	"credential-provider-go/credentials"
	"credential-provider-go/internal"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/golang-jwt/jwt/v5"
)

// sessionNameRegex matches characters NOT allowed in AWS RoleSessionName.
var sessionNameRegex = regexp.MustCompile(`[^\w+=,.@-]`)

// AssumeRoleWithWebIdentity performs direct STS federation without Cognito Identity Pool.
func AssumeRoleWithWebIdentity(cfg *config.ProfileConfig, idToken string, claims jwt.MapClaims) (*credentials.AWSCredentialOutput, error) {
	internal.DebugPrint("Using Direct STS federation (AssumeRoleWithWebIdentity)")

	if cfg.FederatedRoleARN == "" {
		return nil, fmt.Errorf("federated_role_arn is required for direct STS federation")
	}

	// Clear AWS credentials env vars to prevent recursive calls, restore on exit
	saved := clearAWSEnvVars()
	defer restoreEnvVars(saved)

	// Build session name from user identity
	sessionName := buildSessionName(claims)
	internal.DebugPrint("Assuming role: %s", cfg.FederatedRoleARN)
	internal.DebugPrint("Session name: %s", sessionName)

	// Create STS client with no credentials (AssumeRoleWithWebIdentity doesn't need them).
	// Skip shared config/credentials files to avoid "partial credentials" errors when
	// ~/.aws/credentials contains expired placeholder values for any profile.
	ctx := context.Background()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.AWSRegion),
		awsconfig.WithCredentialsProvider(aws.AnonymousCredentials{}),
		awsconfig.WithSharedCredentialsFiles([]string{}),
		awsconfig.WithSharedConfigFiles([]string{}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config: %w", err)
	}

	stsClient := sts.NewFromConfig(awsCfg)

	duration := int32(cfg.MaxSessionDuration)
	if duration == 0 {
		duration = 43200
	}

	input := &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          &cfg.FederatedRoleARN,
		RoleSessionName:  &sessionName,
		WebIdentityToken: &idToken,
		DurationSeconds:  &duration,
	}

	result, err := stsClient.AssumeRoleWithWebIdentity(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to assume role with web identity: %w", err)
	}

	creds := result.Credentials
	expiration := creds.Expiration.UTC().Format("2006-01-02T15:04:05Z")

	internal.DebugPrint("Successfully obtained credentials via Direct STS, expires: %s", expiration)

	return &credentials.AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     *creds.AccessKeyId,
		SecretAccessKey: *creds.SecretAccessKey,
		SessionToken:    *creds.SessionToken,
		Expiration:      expiration,
	}, nil
}

func buildSessionName(claims jwt.MapClaims) string {
	sessionName := "claude-code"

	if sub, ok := claims["sub"].(string); ok && sub != "" {
		truncated := sub
		if len(truncated) > 32 {
			truncated = truncated[:32]
		}
		sanitized := sessionNameRegex.ReplaceAllString(truncated, "-")
		sessionName = "claude-code-" + sanitized
	} else if email, ok := claims["email"].(string); ok && email != "" {
		parts := strings.SplitN(email, "@", 2)
		emailPart := parts[0]
		if len(emailPart) > 32 {
			emailPart = emailPart[:32]
		}
		sanitized := sessionNameRegex.ReplaceAllString(emailPart, "-")
		sessionName = "claude-code-" + sanitized
	}

	return sessionName
}

// clearAWSEnvVars removes AWS credential env vars and returns the saved values.
func clearAWSEnvVars() map[string]string {
	vars := []string{"AWS_PROFILE", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"}
	saved := make(map[string]string)
	for _, v := range vars {
		if val, ok := os.LookupEnv(v); ok {
			saved[v] = val
			os.Unsetenv(v)
		}
	}
	return saved
}

// restoreEnvVars restores previously cleared environment variables.
func restoreEnvVars(saved map[string]string) {
	for k, v := range saved {
		os.Setenv(k, v)
	}
}
