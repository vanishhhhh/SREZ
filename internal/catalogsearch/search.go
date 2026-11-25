package catalogsearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"srez/internal/usecase"
)

type clause map[string]any

func term(field string, value any) clause {
	return clause{"term": clause{field: value}}
}

func query(q usecase.ProductQuery) clause {
	filters := []clause{}
	if q.Category != "" {
		filters = append(filters, term("category", q.Category))
	}
	if q.BrandID != 0 {
		filters = append(filters, term("brand_id", q.BrandID))
	}
	price := clause{}
	if q.MinPrice != nil {
		price["gte"] = *q.MinPrice
	}
	if q.MaxPrice != nil {
		price["lte"] = *q.MaxPrice
	}
	if len(price) != 0 {
		filters = append(filters, clause{"range": clause{"price": price}})
	}
	if q.Size != "" || q.Color != "" {
		variant := []clause{}
		if q.Size != "" {
			variant = append(variant, term("variants.size", q.Size))
		}
		if q.Color != "" {
			variant = append(variant, term("variants.color", q.Color))
		}
		filters = append(filters, clause{"nested": clause{
			"path": "variants", "score_mode": "none", "query": clause{"bool": clause{"filter": variant}},
		}})
	}
	return clause{"bool": clause{"filter": filters}}
}

var ordering = map[string][]clause{
	"id_asc":     {{"id": "asc"}},
	"price_asc":  {{"price": "asc"}, {"id": "asc"}},
	"price_desc": {{"price": "desc"}, {"id": "asc"}},
	"newest":     {{"created_at": "desc"}, {"id": "desc"}},
	"oldest":     {{"created_at": "asc"}, {"id": "asc"}},
}

type searchResult struct {
	TimedOut bool                 `json:"timed_out"`
	Shards   struct{ Failed int } `json:"_shards"`
	Hits     struct {
		Total struct {
			Value    int64
			Relation string
		}
		Hits []struct {
			Source struct{ ID int64 } `json:"_source"`
			Sort   []json.RawMessage
		}
	}
}

// Search takes a normalized query and returns ordered IDs and the exact total.
func (i *Index) Search(ctx context.Context, revisionAlias string, q usecase.ProductQuery) ([]int64, int64, error) {
	offset := (q.Page - 1) * int64(q.Limit)
	body := clause{"query": query(q), "sort": ordering[q.Sort], "_source": []string{"id"}, "track_total_hits": true}
	const resultWindow = 10000
	var total int64
	if offset <= resultWindow-int64(q.Limit) {
		body["from"] = offset
	} else {
		// ES limits from+size. Check the total before advancing the cursor so a
		// very large out-of-range page is still an ordinary empty result.
		body["size"] = 0
		count, err := i.search(ctx, revisionAlias, body)
		if err != nil {
			return nil, 0, err
		}
		total = count.Hits.Total.Value
		if offset >= total {
			return []int64{}, total, nil
		}
		body["track_total_hits"] = false
		for offset > 0 {
			size := min(offset, resultWindow)
			body["size"] = size
			body["_source"] = false
			batch, err := i.search(ctx, revisionAlias, body)
			if err != nil {
				return nil, 0, err
			}
			if int64(len(batch.Hits.Hits)) != size {
				return nil, 0, errors.New("incomplete Elasticsearch page")
			}
			body["search_after"] = batch.Hits.Hits[len(batch.Hits.Hits)-1].Sort
			offset -= size
		}
		body["_source"] = []string{"id"}
	}
	body["size"] = q.Limit
	page, err := i.search(ctx, revisionAlias, body)
	if err != nil {
		return nil, 0, err
	}
	if body["track_total_hits"] == true {
		total = page.Hits.Total.Value
	}
	ids := make([]int64, 0, len(page.Hits.Hits))
	for _, hit := range page.Hits.Hits {
		if hit.Source.ID <= 0 {
			return nil, 0, errors.New("invalid Elasticsearch product ID")
		}
		ids = append(ids, hit.Source.ID)
	}
	return ids, total, nil
}

func (i *Index) search(ctx context.Context, alias string, body clause) (searchResult, error) {
	var result searchResult
	err := i.json(ctx, http.MethodPost, "/"+alias+"/_search?allow_partial_search_results=false", body, &result)
	if err == nil && (result.TimedOut || result.Shards.Failed > 0) {
		err = errors.New("incomplete Elasticsearch result")
	}
	return result, err
}
