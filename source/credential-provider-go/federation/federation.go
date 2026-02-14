package federation

import (
	"credential-provider-go/config"
	"credential-provider-go/credentials"
	"credential-provider-go/internal"

	"github.com/golang-jwt/jwt/v5"
)

// GetAWSCredentials exchanges an OIDC token for AWS credentials using the
// configured federation method (direct STS or Cognito Identity Pool).
func GetAWSCredentials(cfg *config.ProfileConfig, idToken string, claims jwt.MapClaims) (*credentials.AWSCredentialOutput, error) {
	federationType := cfg.FederationType
	if federationType == "" {
		federationType = "cognito"
	}

	internal.DebugPrint("Using federation type: %s", federationType)

	if federationType == "direct" {
		return AssumeRoleWithWebIdentity(cfg, idToken, claims)
	}
	return GetCognitoCredentials(cfg, idToken, claims)
}
