package r2

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Config struct {
	BucketURL       string // optional; required for public bucket
	BucketName      string
	AccountID       string
	AccessKeyID     string
	AccessKeySecret string
}

// R2 Storage using AWS S3 Sdk, Implements [storage.Storage] interface
type R2 struct {
	bucket    string
	publicURL *url.URL
	client    *s3.Client
	isPublic  bool
}

func New(cfg Config) (*R2, error) {
	r2Cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.AccessKeySecret, ""),
		),
		config.WithRegion("auto"),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(r2Cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(
			fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID),
		)
	})

	var publicURL *url.URL
	if cfg.BucketURL != "" {
		u, err := url.Parse(cfg.BucketURL)
		if err != nil {
			return nil, fmt.Errorf("invalid bucket url: %w", err)
		}
		publicURL = u
	}

	return &R2{
		publicURL: publicURL,
		bucket:    cfg.BucketName,
		isPublic:  publicURL != nil,
		client:    client,
	}, nil
}

// Put implements storage.Storage.
func (r *R2) Put(
	ctx context.Context,
	path string,
	reader io.Reader,
	contentType string,
) error {
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(path),
		Body:        reader,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("r2 put: %w", err)
	}
	return nil
}

// Get implements storage.Storage.
func (r *R2) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	result, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, fmt.Errorf("r2 get: %w", err)
	}

	return result.Body, nil
}

// Delete implements storage.Storage.
func (r *R2) Delete(ctx context.Context, path string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return fmt.Errorf("r2 delete: %w", err)
	}

	return nil
}

// Exists implements storage.Storage.
func (r *R2) Exists(ctx context.Context, path string) (bool, error) {
	_, err := r.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		// TODO: Check if it's a "not found" error
		return false, nil
	}
	return true, nil
}

// GetURL returns either:
//   - a direct public URL if the bucket is public
//   - a presigned URL if the bucket is private
func (r *R2) GetURL(ctx context.Context, path string, expiry time.Duration) (string, error) {
	// Case 1: public bucket — just return direct URL
	if r.isPublic && r.publicURL != nil {
		u := *r.publicURL
		u.Path = path
		return u.String(), nil
	}

	// Case 2: private bucket — generate presigned URL
	presignClient := s3.NewPresignClient(r.client)
	presignResult, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(path),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiry
	})
	if err != nil {
		return "", fmt.Errorf("r2 get url: %w", err)
	}

	return presignResult.URL, nil
}

// Dir returns the base URL for a public bucket, or empty for private.
func (r *R2) Dir() string {
	if r.isPublic && r.publicURL != nil {
		return r.publicURL.String()
	}
	return ""
}
