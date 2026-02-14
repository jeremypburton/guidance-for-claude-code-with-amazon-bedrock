package internal

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// DecodeJWTUnverified decodes a JWT token without verifying its signature.
// Returns the claims as a map. This is used for extracting user info from
// ID tokens where signature verification is handled by the OIDC provider.
func DecodeJWTUnverified(tokenString string) (jwt.MapClaims, error) {
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("failed to extract claims from JWT")
	}

	return claims, nil
}
