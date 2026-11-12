package r2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/basarsubasi/website/internal/domain"
)

type Config struct {
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	PublicURL       string
}

type R2 struct {
	client    *s3.Client
	bucket    string
	publicURL string
}

// New creates an S3 client scoped to one bucket.
func New(cfg Config) (*R2, error) {
	if cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("R2 bucket and access credentials are required")
	}
	urls := map[string]string{"endpoint": cfg.Endpoint, "public URL": cfg.PublicURL}
	for name, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("R2 %s must be an absolute HTTP(S) URL without credentials, query, or fragment", name)
		}
	}
	client := s3.NewFromConfig(aws.Config{
		Region:      "auto",
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		HTTPClient:  &http.Client{Timeout: 90 * time.Second},
		// R2 does not implement every S3 flexible-checksum/trailer combination.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}, func(o *s3.Options) { o.BaseEndpoint = aws.String(cfg.Endpoint); o.UsePathStyle = true })
	return &R2{client: client, bucket: cfg.Bucket, publicURL: strings.TrimRight(cfg.PublicURL, "/")}, nil
}

// Image uploads have unique keys and their bytes never change. Metadata edits
// must preserve this policy; catalogs and post backups must not inherit it.
func objectCacheControl(key, contentType string) string {
	if (strings.HasPrefix(key, "photos/") || strings.HasPrefix(key, "images/")) && strings.HasPrefix(contentType, "image/") {
		return "public, max-age=31536000, immutable"
	}
	return "no-store"
}

func (r *R2) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string, metadata map[string]string) (string, error) {
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key), Body: body, ContentLength: aws.Int64(size), ContentType: aws.String(contentType), Metadata: metadata, CacheControl: aws.String(objectCacheControl(key, contentType))})
	if err != nil {
		return "", err
	}
	return r.publicURL + "/" + key, nil
}
func (r *R2) UpdateMetadata(ctx context.Context, key, contentType string, metadata map[string]string) error {
	_, err := r.client.CopyObject(ctx, &s3.CopyObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key), CopySource: aws.String(url.PathEscape(r.bucket + "/" + key)), MetadataDirective: types.MetadataDirectiveReplace, ContentType: aws.String(contentType), Metadata: metadata, CacheControl: aws.String(objectCacheControl(key, contentType))})
	return err
}
func (r *R2) Delete(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)})
	return err
}

const maxObjectJSON = 16 << 20

// Read loads a JSON object and its opaque ETag for conditional catalog writes.
func (r *R2) Read(ctx context.Context, key string) ([]byte, string, error) {
	result, err := r.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, "", objectError(err)
	}
	defer result.Body.Close()
	data, err := io.ReadAll(io.LimitReader(result.Body, maxObjectJSON+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxObjectJSON {
		return nil, "", fmt.Errorf("JSON object exceeds 16 MiB")
	}
	return data, aws.ToString(result.ETag), nil
}
func (r *R2) PutIf(ctx context.Context, key string, data []byte, etag string, create bool) (string, error) {
	input := &s3.PutObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentLength: aws.Int64(int64(len(data))), ContentType: aws.String("application/json"), CacheControl: aws.String("no-store")}
	if create {
		input.IfNoneMatch = aws.String("*")
	} else {
		input.IfMatch = aws.String(etag)
	}
	// A catalog write has exactly one attempt; a timeout is reconciled by revision.
	result, err := r.client.PutObject(ctx, input, func(o *s3.Options) { o.RetryMaxAttempts = 1 })
	if err != nil {
		return "", objectError(err)
	}
	return aws.ToString(result.ETag), nil
}
func objectError(err error) error {
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "NoSuchKey", "NotFound":
			return domain.ErrNotFound
		case "PreconditionFailed", "ConditionalRequestConflict":
			return domain.ErrConflict
		}
	}
	return err
}
