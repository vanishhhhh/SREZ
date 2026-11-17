// qa-media-copy is an opt-in operator tool, never an HTTP endpoint. It reads
// the working S3 store and copies only absent referenced objects to QA.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/lib/pq"
)

func main() {
	if err := copyObjects(); err != nil {
		fmt.Fprintln(os.Stderr, "QA media copy failed. Source objects and existing QA objects were not overwritten.")
		os.Exit(1)
	}
}
func copyObjects() error {
	if os.Getenv("SREZ_QA_MEDIA_COPY") != "1" {
		return errors.New("explicit QA opt-in required")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || u.Host != "127.0.0.1:15433" || u.Path != "/srez" {
		return errors.New("QA database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT a.object_key,a.mime,a.size_bytes FROM media_assets a WHERE a.state='ready' AND EXISTS(SELECT 1 FROM media_references r WHERE r.asset_id=a.id) ORDER BY a.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type object struct {
		key, mime string
		size      int64
	}
	objects := []object{}
	for rows.Next() {
		var o object
		if err = rows.Scan(&o.key, &o.mime, &o.size); err != nil {
			return err
		}
		objects = append(objects, o)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	cfg := aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider(os.Getenv("SREZ_S3_ACCESS_KEY"), os.Getenv("SREZ_S3_SECRET_KEY"), ""), HTTPClient: &http.Client{Timeout: 20 * time.Second}, RetryMaxAttempts: 2, RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired}
	client := func(endpoint string) *s3.Client {
		return s3.NewFromConfig(cfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(endpoint); o.UsePathStyle = true })
	}
	source, target := client("http://127.0.0.1:18333"), client("http://127.0.0.1:18334")
	bucket := "srez-media"
	copied, existing, missing := 0, 0, 0
	var total int64
	for _, o := range objects {
		_, e := target.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &bucket, Key: &o.key})
		if e == nil {
			existing++
			continue
		}
		var status interface{ HTTPStatusCode() int }
		if !errors.As(e, &status) || status.HTTPStatusCode() != 404 {
			return errors.New("QA object lookup failed")
		}
		remote, e := source.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &o.key})
		if e != nil {
			missing++
			continue
		}
		body, e := io.ReadAll(io.LimitReader(remote.Body, 8*1024*1024+1))
		remote.Body.Close()
		if e != nil || int64(len(body)) != o.size || len(body) > 8*1024*1024 {
			return errors.New("source object integrity mismatch")
		}
		_, e = target.PutObject(ctx, &s3.PutObjectInput{Bucket: &bucket, Key: &o.key, ContentType: &o.mime, Body: bytes.NewReader(body), IfNoneMatch: aws.String("*")})
		if e != nil {
			return errors.New("QA conditional copy failed")
		}
		check, e := target.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &o.key})
		if e != nil {
			return errors.New("QA verification failed")
		}
		verified, e := io.ReadAll(io.LimitReader(check.Body, 8*1024*1024+1))
		check.Body.Close()
		if e != nil || sha256.Sum256(body) != sha256.Sum256(verified) {
			return errors.New("QA copy digest mismatch")
		}
		copied++
		total += int64(len(body))
	}
	result := struct {
		SourceReadonly, ExistingQAObjectsPreserved, HashesVerified bool
		Copied, Existing, MissingSource                            int
		Bytes                                                      int64
	}{true, true, true, copied, existing, missing, total}
	json.NewEncoder(os.Stdout).Encode(result)
	if missing > 0 {
		return errors.New("some referenced source objects are missing")
	}
	return nil
}
