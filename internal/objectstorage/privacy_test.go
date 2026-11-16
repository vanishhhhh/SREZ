package objectstorage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestPrivatePolicyPermissionAnalysis(t *testing.T) {
	for _, tc := range []struct {
		name, statement string
		safe            bool
	}{
		{"public read", `{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"*"}`, false},
		{"public list", `{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":"s3:ListBucket","Resource":"*"}`, false},
		{"conditional public", `{"Effect":"Allow","Principal":"*","Action":"s3:*","Condition":{"IpAddress":{"aws:SourceIp":"127.0.0.1"}}}`, false},
		{"not principal", `{"Effect":"Allow","NotPrincipal":{"AWS":"arn:aws:iam::123:user/a"},"Action":"s3:*"}`, false},
		{"named IAM", `{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::123456789012:user/backend"},"Action":"s3:GetObject","Resource":"*"}`, true},
		{"deny", `{"Effect":"Deny","Principal":"*","Action":"s3:PutObject","Resource":"*"}`, true},
		{"unknown", `{"Effect":"Maybe","Principal":"*"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var st any
			if e := json.Unmarshal([]byte(tc.statement), &st); e != nil {
				t.Fatal(e)
			}
			if (privateStatements([]any{st}) == nil) != tc.safe {
				t.Fatal("incorrect permissions decision")
			}
		})
	}
}

func TestSeaweedBucketPrivacyGuard(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("isolated S3 required")
	}
	s, e := New(Config{Endpoint: endpoint, PublicEndpoint: endpoint, Region: "us-east-1", Bucket: "privacy-regression", AccessKey: os.Getenv("SREZ_S3_ACCESS_KEY"), SecretKey: os.Getenv("SREZ_S3_SECRET_KEY")})
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	if e = s.EnsureBucket(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.RetireStaging(ctx); e != nil {
		t.Fatal("fresh private bucket rejected", e)
	}
	if e = s.RetireStaging(ctx); e != nil {
		t.Fatal("idempotent private bucket rejected", e)
	}
	if e = s.Put(ctx, "privacy-probe", "text/plain", strings.NewReader("privacy control")); e != nil {
		t.Fatal(e)
	}
	anonymous := func(path string, want int) {
		t.Helper()
		res, err := http.Get(endpoint + "/" + s.bucket + path)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("anonymous permissions: got %d want %d", res.StatusCode, want)
		}
	}
	anonymous("/privacy-probe", 403)
	anonymous("?list-type=2", 403)
	for _, action := range []string{"s3:GetObject", "s3:ListBucket"} {
		p := `{"Version":"2012-10-17","Statement":[{"Sid":"ExternalPublic","Effect":"Allow","Principal":"*","Action":"` + action + `","Resource":["arn:aws:s3:::privacy-regression","arn:aws:s3:::privacy-regression/*"]}]}`
		if _, e = s.client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{Bucket: &s.bucket, Policy: &p}); e != nil {
			t.Fatal(e)
		}
		if action == "s3:GetObject" {
			anonymous("/privacy-probe", 200)
		} else {
			anonymous("?list-type=2", 200)
		}
		before, err := s.client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: &s.bucket})
		if err != nil {
			t.Fatal(err)
		}
		if e = s.RetireStaging(ctx); !errors.Is(e, ErrUnsafeBucket) {
			t.Fatal("public bucket initialization accepted", action, e)
		}
		got, e := s.client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: &s.bucket})
		if e != nil || aws.ToString(got.Policy) != aws.ToString(before.Policy) {
			t.Fatal("external unsafe policy was rewritten", e)
		}
	}
	if _, e = s.client.DeleteBucketPolicy(ctx, &s3.DeleteBucketPolicyInput{Bucket: &s.bucket}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.client.PutBucketAcl(ctx, &s3.PutBucketAclInput{Bucket: &s.bucket, ACL: types.BucketCannedACLPublicRead}); e != nil {
		t.Fatal("pinned provider public ACL control failed", safe(e))
	}
	anonymous("?list-type=2", 200)
	if e = s.RetireStaging(ctx); !errors.Is(e, ErrUnsafeBucket) {
		t.Fatal("public ACL accepted", e)
	}
	if _, e = s.client.PutBucketAcl(ctx, &s3.PutBucketAclInput{Bucket: &s.bucket, ACL: types.BucketCannedACLPrivate}); e != nil {
		t.Fatal(e)
	}
	if e = s.RetireStaging(ctx); e != nil {
		t.Fatal("private ACL rejected", e)
	}
	anonymous("?list-type=2", 403)
	private := `{"Version":"2012-10-17","Statement":[{"Sid":"ExternalPrivate","Effect":"Deny","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::privacy-regression/external/*"}]}`
	if _, e = s.client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{Bucket: &s.bucket, Policy: &private}); e != nil {
		t.Fatal(e)
	}
	if e = s.RetireStaging(ctx); e != nil {
		t.Fatal("existing private policy rejected", e)
	}
	got, e := s.client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: &s.bucket})
	if e != nil || !strings.Contains(aws.ToString(got.Policy), "ExternalPrivate") || !strings.Contains(aws.ToString(got.Policy), "SREZRetiredStagingUploads") {
		t.Fatal("private statement or staging deny lost", e)
	}
}

// A provider that cannot expose authoritative ACL metadata must not be treated
// as private; startup must stop before any policy mutation.
func TestPrivacyInspectionFailureIsFailClosed(t *testing.T) {
	for _, status := range []int{403, 501, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				w.WriteHeader(status)
				if status == 200 {
					io.WriteString(w, `<AccessControlPolicy xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></AccessControlPolicy>`)
				}
			}))
			defer server.Close()
			s, e := New(Config{Endpoint: server.URL, PublicEndpoint: server.URL, Region: "us-east-1", Bucket: "private", AccessKey: "test", SecretKey: "test"})
			if e != nil {
				t.Fatal(e)
			}
			if e = s.RetireStaging(context.Background()); !errors.Is(e, ErrUnsafeBucket) || writes != 0 {
				t.Fatal("unverifiable ACL allowed startup/mutation", e, writes)
			}
		})
	}
}
