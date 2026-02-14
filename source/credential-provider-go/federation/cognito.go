package federation

import (
	"context"
	"fmt"
	"strings"

	"credential-provider-go/config"
	"credential-provider-go/credentials"
	"credential-provider-go/internal"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentity"
	"github.com/golang-jwt/jwt/v5"
)

// GetCognitoCredentials exchanges an OIDC token for AWS credentials
// via Cognito Identity Pool (GetId + GetCredentialsForIdentity).
func GetCognitoCredentials(cfg *config.ProfileConfig, idToken string, claims jwt.MapClaims) (*credentials.AWSCredentialOutput, error) {
	internal.DebugPrint("Using Cognito Identity Pool federation")

	// Clear AWS credentials env vars to prevent recursive calls, restore on exit
	saved := clearAWSEnvVars()
	defer restoreEnvVars(saved)

	ctx := context.Background()

	// Create Cognito Identity client with anonymous credentials (no AWS creds needed)
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.AWSRegion),
		awsconfig.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config: %w", err)
	}

	cognitoClient := cognitoidentity.NewFromConfig(awsCfg)

	// Determine the login key based on provider type
	loginKey := determineLoginKey(cfg, claims)
	internal.DebugPrint("Login key: %s", loginKey)
	internal.DebugPrint("Identity Pool ID: %s", cfg.IdentityPoolID)

	// Get Cognito identity
	internal.DebugPrint("Calling GetId with identity pool: %s", cfg.IdentityPoolID)
	idResp, err := cognitoClient.GetId(ctx, &cognitoidentity.GetIdInput{
		IdentityPoolId: &cfg.IdentityPoolID,
		Logins:         map[string]string{loginKey: idToken},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get Cognito identity: %w", err)
	}

	identityID := *idResp.IdentityId
	internal.DebugPrint("Got Cognito Identity ID: %s", identityID)

	// Get credentials for identity
	credsResp, err := cognitoClient.GetCredentialsForIdentity(ctx, &cognitoidentity.GetCredentialsForIdentityInput{
		IdentityId: &identityID,
		Logins:     map[string]string{loginKey: idToken},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials for identity: %w", err)
	}

	creds := credsResp.Credentials
	expiration := creds.Expiration.UTC().Format("2006-01-02T15:04:05Z")

	// Note: Cognito returns "SecretKey" but AWS CLI expects "SecretAccessKey"
	return &credentials.AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     *creds.AccessKeyId,
		SecretAccessKey: *creds.SecretKey,
		SessionToken:    *creds.SessionToken,
		Expiration:      expiration,
	}, nil
}

func determineLoginKey(cfg *config.ProfileConfig, claims jwt.MapClaims) string {
	// For Cognito User Pool, extract from token issuer to ensure case matches
	if cfg.ProviderType == "cognito" {
		if iss, ok := claims["iss"].(string); ok && iss != "" {
			internal.DebugPrint("Using issuer from token as login key")
			return strings.TrimPrefix(iss, "https://")
		}
		// Fallback: construct from config
		if cfg.CognitoUserPoolID != "" {
			return fmt.Sprintf("cognito-idp.%s.amazonaws.com/%s", cfg.AWSRegion, cfg.CognitoUserPoolID)
		}
	}

	// For external OIDC providers, use the provider domain
	return cfg.ProviderDomain
}
