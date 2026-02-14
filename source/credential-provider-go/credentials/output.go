package credentials

import (
	"encoding/json"
	"fmt"
)

// AWSCredentialOutput is the JSON structure expected by AWS CLI's credential_process.
type AWSCredentialOutput struct {
	Version         int    `json:"Version"`
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	SessionToken    string `json:"SessionToken"`
	Expiration      string `json:"Expiration"`
}

// PrintCredentials serializes credentials to JSON and writes to stdout.
func PrintCredentials(creds *AWSCredentialOutput) error {
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
