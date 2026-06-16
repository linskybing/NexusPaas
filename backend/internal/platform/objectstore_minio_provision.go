package platform

import (
	"context"
	"fmt"
	"strings"

	"github.com/minio/minio-go/v7"
)

// EnsureObjectStoreBucket creates the configured object-store bucket when it is
// absent and succeeds without changes when the bucket already exists.
func EnsureObjectStoreBucket(ctx context.Context, endpointURL, accessKey, secretKey, bucket string) (bool, error) {
	client, err := newMinioClient(endpointURL, accessKey, secretKey)
	if err != nil {
		return false, err
	}
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return false, fmt.Errorf("OBJECT_STORE_BUCKET is required")
	}
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return false, fmt.Errorf("check object store bucket: %w", err)
	}
	if exists {
		return false, nil
	}
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		return false, fmt.Errorf("create object store bucket: %w", err)
	}
	return true, nil
}
