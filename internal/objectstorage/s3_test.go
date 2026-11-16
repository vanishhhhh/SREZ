package objectstorage

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestBrowserEndpointSignature(t *testing.T) {
	s, e := New(Config{Endpoint: "http://seaweedfs:8333", PublicEndpoint: "http://127.0.0.1:8333", Region: "us-east-1", Bucket: "srez-media", AccessKey: "test-access", SecretKey: "test-secret"})
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.Presign(context.Background(), "staging/random", "image/jpeg")
	if e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(p.URL)
	if e != nil {
		t.Fatal(e)
	}
	if u.Host != "127.0.0.1:8333" || u.Path != "/srez-media/staging/random" || u.Query().Get("X-Amz-Expires") != "120" || !strings.Contains(u.Query().Get("X-Amz-SignedHeaders"), "host") || !strings.Contains(u.Query().Get("X-Amz-SignedHeaders"), "content-type") || strings.Contains(p.URL, "test-secret") {
		t.Fatal("incorrect signature configuration")
	}
	if p.Headers["Content-Type"] != "image/jpeg" {
		t.Fatal("missing content type")
	}
}
func TestKeysAndConfiguration(t *testing.T) {
	a, _ := Key("staging")
	b, _ := Key("staging")
	if a == b || len(a) != 72 {
		t.Fatal("weak key")
	}
	if _, e := New(Config{}); e == nil {
		t.Fatal("empty config allowed")
	}
}
