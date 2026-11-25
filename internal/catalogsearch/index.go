// Package catalogsearch stores PostgreSQL catalog snapshots in Elasticsearch.
package catalogsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrMissingRevision = errors.New("catalog index revision is missing")

type Document struct {
	ID        int64     `json:"id"`
	BrandID   int64     `json:"brand_id"`
	Category  string    `json:"category"`
	Price     int64     `json:"price"`
	CreatedAt int64     `json:"created_at"`
	Variants  []Variant `json:"variants"`
}

type Variant struct {
	Size  string `json:"size"`
	Color string `json:"color"`
}

type Index struct {
	endpoint string
	username string
	password string
	client   *http.Client
}

func New(endpoint, username, password string) (*Index, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || (username == "") != (password == "") {
		return nil, errors.New("invalid Elasticsearch configuration")
	}
	return &Index{
		endpoint: strings.TrimRight(endpoint, "/"), username: username, password: password,
		client: &http.Client{
			Timeout:       15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (i *Index) request(ctx context.Context, method, path, contentType string, body io.Reader, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, i.endpoint+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if i.username != "" {
		req.SetBasicAuth(i.username, i.password)
	}
	res, err := i.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return ErrMissingRevision
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Elasticsearch returned HTTP %d", res.StatusCode)
	}
	if result != nil {
		return json.NewDecoder(res.Body).Decode(result)
	}
	_, err = io.Copy(io.Discard, res.Body)
	return err
}

const mapping = `{
  "settings":{"number_of_shards":1,"number_of_replicas":0},
  "mappings":{"dynamic":"strict","properties":{
    "id":{"type":"long"},"brand_id":{"type":"long"},"category":{"type":"keyword"},
    "price":{"type":"long"},"created_at":{"type":"long"},
    "variants":{"type":"nested","properties":{"size":{"type":"keyword"},"color":{"type":"keyword"}}}
  }}
}`

// Replace builds a private generation before making it searchable. The caller
// serializes publishers for this PostgreSQL catalog.
func (i *Index) Replace(ctx context.Context, alias, revisionAlias string, docs []Document) error {
	name := revisionAlias + "-build"
	var previous map[string]json.RawMessage
	err := i.request(ctx, http.MethodGet, "/_alias/"+alias, "", nil, &previous)
	if err != nil && !errors.Is(err, ErrMissingRevision) {
		return err
	}
	// A failed or interrupted build can leave this exact generation behind.
	if _, active := previous[name]; active {
		return errors.New("catalog generation is already active")
	}
	if err = i.request(ctx, http.MethodDelete, "/"+name, "", nil, nil); err != nil && !errors.Is(err, ErrMissingRevision) {
		return err
	}
	if err = i.request(ctx, http.MethodPut, "/"+name, "application/json", strings.NewReader(mapping), nil); err != nil {
		return err
	}
	for start := 0; start < len(docs); start += 500 {
		if err = i.bulk(ctx, name, docs[start:min(start+500, len(docs))]); err != nil {
			return err
		}
	}
	if err = i.request(ctx, http.MethodPost, "/"+name+"/_refresh", "", nil, nil); err != nil {
		return err
	}
	actions := []map[string]map[string]string{
		{"add": {"index": name, "alias": alias}},
		{"add": {"index": name, "alias": revisionAlias}},
	}
	for old := range previous {
		if !strings.HasPrefix(old, alias+"-r") {
			return errors.New("catalog alias points to an unexpected index")
		}
		actions = append(actions, map[string]map[string]string{"remove_index": {"index": old}})
	}
	var result struct {
		Acknowledged bool `json:"acknowledged"`
		Errors       bool `json:"errors"`
	}
	if err = i.json(ctx, http.MethodPost, "/_aliases", map[string]any{"actions": actions}, &result); err != nil {
		return err
	}
	if !result.Acknowledged || result.Errors {
		return errors.New("catalog alias update failed")
	}
	return nil
}

func (i *Index) bulk(ctx context.Context, name string, docs []Document) error {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	for _, doc := range docs {
		if err := encoder.Encode(map[string]any{"index": map[string]string{"_id": fmt.Sprint(doc.ID)}}); err != nil {
			return err
		}
		if err := encoder.Encode(doc); err != nil {
			return err
		}
	}
	var result struct{ Errors bool }
	if err := i.request(ctx, http.MethodPost, "/"+name+"/_bulk", "application/x-ndjson", &body, &result); err != nil {
		return err
	}
	if result.Errors {
		return errors.New("catalog bulk indexing failed")
	}
	return nil
}

func (i *Index) json(ctx context.Context, method, path string, body, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return i.request(ctx, method, path, "application/json", bytes.NewReader(data), result)
}
