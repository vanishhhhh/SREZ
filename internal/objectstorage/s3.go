// Package objectstorage implements private, path-style S3 access. Never log SDK
// errors or presigned URLs: both may contain credentials/signatures.
package objectstorage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	"srez/internal/usecase"
)

type Config struct{ Endpoint, PublicEndpoint, Region, Bucket, AccessKey, SecretKey string }
type S3 struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func New(c Config) (*S3, error) {
	for _, raw := range []string{c.Endpoint, c.PublicEndpoint} {
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || (u.Path != "" && u.Path != "/") {
			return nil, errors.New("invalid S3 endpoint")
		}
	}
	if c.AccessKey == "" || c.SecretKey == "" || c.Bucket == "" || c.Region == "" {
		return nil, errors.New("missing S3 configuration")
	}
	cfg := aws.Config{Region: c.Region, Credentials: credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, ""), HTTPClient: &http.Client{Timeout: 20 * time.Second}, RetryMaxAttempts: 2, RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired}
	makeClient := func(endpoint string) *s3.Client {
		return s3.NewFromConfig(cfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(endpoint); o.UsePathStyle = true })
	}
	return &S3{makeClient(c.Endpoint), s3.NewPresignClient(makeClient(c.PublicEndpoint)), c.Bucket}, nil
}
func Key(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return prefix + "/" + hex.EncodeToString(b), nil
}

var errStorage = errors.New("object storage operation failed")

func safe(e error) error {
	if e != nil {
		var status interface{ HTTPStatusCode() int }
		if errors.As(e, &status) {
			return fmt.Errorf("object storage operation failed (HTTP %d)", status.HTTPStatusCode())
		}
		return errStorage
	}
	return nil
}
func (s *S3) Ready(ctx context.Context) error {
	_, e := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &s.bucket})
	return safe(e)
}
func (s *S3) EnsureBucket(ctx context.Context) error {
	_, e := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &s.bucket})
	if e == nil {
		return nil
	}
	var status interface{ HTTPStatusCode() int }
	if !errors.As(e, &status) || status.HTTPStatusCode() != 404 {
		return errStorage
	}
	_, e = s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &s.bucket})
	return safe(e)
}
func (s *S3) Presign(ctx context.Context, key, mime string) (usecase.UploadURL, error) {
	p, e := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, ContentType: &mime}, s3.WithPresignExpires(2*time.Minute))
	if e != nil {
		return usecase.UploadURL{}, errStorage
	}
	headers := map[string]string{"Content-Type": mime}
	for k, v := range p.SignedHeader {
		if !strings.EqualFold(k, "Host") {
			headers[k] = strings.Join(v, ",")
		}
	}
	return usecase.UploadURL{URL: p.URL, Headers: headers, ExpiresAt: time.Now().UTC().Add(2 * time.Minute)}, nil
}
func (s *S3) Head(ctx context.Context, key string) (usecase.ObjectInfo, error) {
	v, e := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if e != nil {
		return usecase.ObjectInfo{}, safe(e)
	}
	return usecase.ObjectInfo{Size: aws.ToInt64(v.ContentLength), MIME: aws.ToString(v.ContentType)}, nil
}
func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	v, e := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if e != nil {
		return nil, safe(e)
	}
	return v.Body, nil
}
func (s *S3) Put(ctx context.Context, key, mime string, body io.Reader) error {
	_, e := s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, ContentType: &mime, Body: body})
	return safe(e)
}
func (s *S3) Copy(ctx context.Context, source, target string) error {
	src := url.PathEscape(s.bucket + "/" + source)
	_, e := s.client.CopyObject(ctx, &s3.CopyObjectInput{Bucket: &s.bucket, Key: &target, CopySource: &src})
	return safe(e)
}
func (s *S3) Delete(ctx context.Context, key string) error {
	_, e := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
	return safe(e)
}

// RetireStaging revokes even unexpired Phase 1 presigned PUTs. Preserve other
// bucket-policy statements; only this application's named statement is owned.
// Failure is fail-closed at startup, never a fallback to accepting old uploads.
func (s *S3) RetireStaging(ctx context.Context) error {
	if e := s.verifyACL(ctx); e != nil {
		return e
	}
	policy := map[string]any{"Version": "2012-10-17"}
	existing, e := s.client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: &s.bucket})
	if e == nil {
		if e = json.Unmarshal([]byte(aws.ToString(existing.Policy)), &policy); e != nil {
			return errStorage
		}
	} else {
		var status interface{ HTTPStatusCode() int }
		if !errors.As(e, &status) || status.HTTPStatusCode() != 404 {
			return safe(e)
		}
	}
	if policy == nil {
		return errStorage
	}
	const sid = "SREZRetiredStagingUploads"
	statements := []any{}
	switch value := policy["Statement"].(type) {
	case nil:
	case []any:
		statements = value
	case map[string]any:
		statements = append(statements, value)
	default:
		return errStorage
	}
	if e := privateStatements(statements); e != nil {
		return e
	}
	kept := make([]any, 0, len(statements)+1)
	for _, item := range statements {
		v, ok := item.(map[string]any)
		if !ok {
			return errStorage
		}
		if v["Sid"] != sid {
			kept = append(kept, item)
		}
	}
	kept = append(kept, map[string]any{"Sid": sid, "Effect": "Deny", "Principal": "*", "Action": "s3:PutObject", "Resource": "arn:aws:s3:::" + s.bucket + "/staging/*"})
	policy["Statement"] = kept
	body, e := json.Marshal(policy)
	if e != nil {
		return errStorage
	}
	_, e = s.client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{Bucket: &s.bucket, Policy: aws.String(string(body))})
	return safe(e)
}
