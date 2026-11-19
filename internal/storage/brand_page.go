package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"srez/internal/entities"
)

func readBrandPageConfig(raw []byte) (*entities.BrandPageConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var config entities.BrandPageConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &config, nil
}

func (m *managementTx) BrandPage(ctx context.Context) (*entities.BrandPageEditorState, error) {
	var draftJSON, publishedJSON []byte
	var state entities.BrandPageEditorState
	err := m.tx.QueryRowContext(ctx, `SELECT revision, draft, published, published_revision FROM brand_pages WHERE brand_id=$1`, m.brandID).
		Scan(&state.Revision, &draftJSON, &publishedJSON, &state.PublishedRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	draft, err := readBrandPageConfig(draftJSON)
	if err != nil {
		return nil, err
	}
	state.Draft = *draft
	state.Published, err = readBrandPageConfig(publishedJSON)
	return &state, err
}

func (m *managementTx) SaveBrandPage(ctx context.Context, state entities.BrandPageEditorState) error {
	draft, err := json.Marshal(state.Draft)
	if err != nil {
		return err
	}
	var published any
	if state.Published != nil {
		snapshot, err := json.Marshal(state.Published)
		if err != nil {
			return err
		}
		published = string(snapshot)
	}
	_, err = m.tx.ExecContext(ctx, `
		INSERT INTO brand_pages(brand_id, revision, draft, published, published_revision)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (brand_id) DO UPDATE SET revision=EXCLUDED.revision, draft=EXCLUDED.draft,
			published=EXCLUDED.published, published_revision=EXCLUDED.published_revision`,
		m.brandID, state.Revision, string(draft), published, state.PublishedRevision)
	if err != nil {
		return err
	}
	return m.replaceBrandPageMedia(ctx, state)
}
