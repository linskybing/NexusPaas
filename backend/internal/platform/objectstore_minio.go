package platform

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// minioObjectStore is the production ObjectStore backed by an S3-compatible
// endpoint (MinIO or AWS S3). It is constructed from OBJECT_STORE_* config in
// NewBackingResources and injected via WithObjectStore.
type minioObjectStore struct {
	client *minio.Client
	bucket string
}

func newMinioClient(endpointURL, accessKey, secretKey string) (*minio.Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpointURL))
	if err != nil {
		return nil, fmt.Errorf("parse OBJECT_STORE_URL: %w", err)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("OBJECT_STORE_URL must be an absolute http(s) URL")
	}
	var secure bool
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		secure = false
	case "https":
		secure = true
	default:
		return nil, fmt.Errorf("OBJECT_STORE_URL must be an http:// or https:// URL")
	}
	client, err := minio.New(parsed.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("connect object store: %w", err)
	}
	return client, nil
}

// NewMinioObjectStore connects to the S3-compatible endpoint described by
// endpointURL (http://host:port or https://host:port), verifies the bucket
// exists, and returns the ObjectStore. Serving runtime must not provision
// buckets; run ADMIN_TASK=ensure-object-store-bucket before startup.
func NewMinioObjectStore(ctx context.Context, endpointURL, accessKey, secretKey, bucket string) (ObjectStore, error) {
	client, err := newMinioClient(endpointURL, accessKey, secretKey)
	if err != nil {
		return nil, err
	}
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, fmt.Errorf("OBJECT_STORE_BUCKET is required")
	}
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check object store bucket: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("object store bucket %q does not exist; run ADMIN_TASK=ensure-object-store-bucket before starting serving runtime", bucket)
	}
	return &minioObjectStore{client: client, bucket: bucket}, nil
}

func (s *minioObjectStore) Put(ctx context.Context, key string, body []byte, contentType string) error {
	if key == "" {
		return fmt.Errorf("object key is required")
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

func (s *minioObjectStore) PutStream(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if key == "" {
		return fmt.Errorf("object key is required")
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("put object stream: %w", err)
	}
	return nil
}

func (s *minioObjectStore) Get(ctx context.Context, key string) ([]byte, string, bool, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", false, fmt.Errorf("get object: %w", err)
	}
	defer obj.Close()
	body, err := io.ReadAll(obj)
	if err != nil {
		if isMinioNotFound(err) {
			return nil, "", false, nil
		}
		return nil, "", false, fmt.Errorf("read object: %w", err)
	}
	info, err := obj.Stat()
	if err != nil {
		if isMinioNotFound(err) {
			return nil, "", false, nil
		}
		return nil, "", false, fmt.Errorf("stat object: %w", err)
	}
	return body, info.ContentType, true, nil
}

func (s *minioObjectStore) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func (s *minioObjectStore) List(ctx context.Context) ([]ObjectInfo, error) {
	infos := []ObjectInfo{}
	for object := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Recursive: true}) {
		if object.Err != nil {
			return nil, fmt.Errorf("list objects: %w", object.Err)
		}
		infos = append(infos, ObjectInfo{Key: object.Key, Size: object.Size, ContentType: object.ContentType})
	}
	return infos, nil
}

func (s *minioObjectStore) HealthCheck(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("object store unreachable: %w", err)
	}
	if !exists {
		return fmt.Errorf("object store bucket %q does not exist", s.bucket)
	}
	return nil
}

func isMinioNotFound(err error) bool {
	return minio.ToErrorResponse(err).Code == "NoSuchKey"
}

var _ ObjectStore = (*minioObjectStore)(nil)
