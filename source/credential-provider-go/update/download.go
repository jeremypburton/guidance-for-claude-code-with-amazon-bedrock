package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"credential-provider-go/internal"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	manifestTimeout = 10 * time.Second
	downloadTimeout = 5 * time.Minute
	maxZipSize      = 100 * 1024 * 1024 // 100 MB
)

// manifest represents the latest.json file in S3.
type manifest struct {
	Version    string                    `json:"version"`
	ReleasedAt string                    `json:"released_at"`
	Binaries   map[string]binaryDetails  `json:"binaries"`
}

// binaryDetails describes one platform binary in the manifest.
type binaryDetails struct {
	S3Key     string `json:"s3_key"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

// newS3Client creates an S3 client using static credentials to avoid
// recursive credential_process invocation. Shared config/credentials files are
// skipped to prevent "partial credentials" errors from expired placeholder values.
func newS3Client(region, accessKeyID, secretKey, sessionToken string) (*s3.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretKey, sessionToken),
		),
		awsconfig.WithSharedCredentialsFiles([]string{}),
		awsconfig.WithSharedConfigFiles([]string{}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config: %w", err)
	}
	return s3.NewFromConfig(cfg), nil
}

// fetchManifest downloads and parses the latest.json manifest from S3.
func fetchManifest(client *s3.Client, bucket, prefix string) (*manifest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), manifestTimeout)
	defer cancel()

	key := prefix + "/latest.json"
	resp, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB max for manifest
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &m, nil
}

// downloadZip downloads the platform zip from S3, validates size and checksum.
// Returns the path to the downloaded temporary file.
func downloadZip(client *s3.Client, bucket string, details binaryDetails) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	resp, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(details.S3Key),
	})
	if err != nil {
		return "", fmt.Errorf("failed to download zip: %w", err)
	}
	defer resp.Body.Close()

	// Validate Content-Length matches expected size
	if resp.ContentLength != nil && details.SizeBytes > 0 {
		if *resp.ContentLength != details.SizeBytes {
			return "", fmt.Errorf("content-length mismatch: expected %d, got %d",
				details.SizeBytes, *resp.ContentLength)
		}
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "ccwb-update-*.zip")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Download with size limit and checksum computation
	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)
	reader := io.LimitReader(resp.Body, maxZipSize+1) // +1 to detect overflow

	n, err := io.Copy(writer, reader)
	tmpFile.Close()

	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write zip: %w", err)
	}

	if n > maxZipSize {
		os.Remove(tmpPath)
		return "", fmt.Errorf("zip file exceeds maximum size of %d bytes", maxZipSize)
	}

	// Validate size matches manifest
	if details.SizeBytes > 0 && n != details.SizeBytes {
		os.Remove(tmpPath)
		return "", fmt.Errorf("download size mismatch: expected %d, got %d", details.SizeBytes, n)
	}

	// Validate checksum
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if details.SHA256 != "" && actualChecksum != details.SHA256 {
		os.Remove(tmpPath)
		return "", fmt.Errorf("checksum mismatch: expected %s, got %s", details.SHA256, actualChecksum)
	}

	internal.DebugPrint("Downloaded %d bytes, checksum verified", n)
	return tmpPath, nil
}
