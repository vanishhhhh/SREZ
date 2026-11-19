package usecase

import (
	"context"
	"errors"

	"srez/internal/entities"
)

var ErrBrandPageWritesLimited = errors.New("brand page version is read-only during rollout")

// LimitBrandPageWrites is applied once at startup, before routes are served.
// Read support remains independent of the write gate.
func (m *Management) LimitBrandPageWrites(version int) error {
	if version < entities.BrandPageVersion || version > entities.BrandPageCompositionVersion {
		return entities.ErrInvalidValue
	}
	m.brandPageWriteVersion = version
	return nil
}

func (m *Management) EditBrandPage(ctx context.Context, uid, id int64) (entities.BrandPageEditorState, error) {
	return m.brandPage(ctx, uid, id, nil)
}

func (m *Management) SaveBrandPage(ctx context.Context, uid, id, revision int64, draft entities.BrandPageConfig) (entities.BrandPageEditorState, error) {
	if err := draft.Validate(); err != nil {
		return entities.BrandPageEditorState{}, err
	}
	return m.updateBrandPage(ctx, uid, id, revision, func(page *entities.BrandPageEditorState) error {
		if draft.SchemaVersion > m.brandPageWriteVersion {
			return ErrBrandPageWritesLimited
		}
		page.Draft = draft
		return nil
	})
}

func (m *Management) PublishBrandPage(ctx context.Context, uid, id, revision int64) (entities.BrandPageEditorState, error) {
	return m.updateBrandPage(ctx, uid, id, revision, func(page *entities.BrandPageEditorState) error {
		if revision == 0 {
			return ErrConflict
		}
		if err := page.Draft.Validate(); err != nil {
			return err
		}
		if page.Draft.SchemaVersion > m.brandPageWriteVersion {
			return ErrBrandPageWritesLimited
		}
		publishedRevision := page.Revision
		page.PublishedRevision = &publishedRevision
		snapshot := page.Draft
		page.Published = &snapshot
		return nil
	})
}

func (m *Management) updateBrandPage(ctx context.Context, uid, id, revision int64, change func(*entities.BrandPageEditorState) error) (entities.BrandPageEditorState, error) {
	return m.brandPage(ctx, uid, id, func(page *entities.BrandPageEditorState) error {
		if page.Revision != revision || revision >= entities.MaxBrandPageRevision {
			return ErrConflict
		}
		page.Revision++
		return change(page)
	})
}

func (m *Management) brandPage(ctx context.Context, uid, id int64, change func(*entities.BrandPageEditorState) error) (entities.BrandPageEditorState, error) {
	var page entities.BrandPageEditorState
	err := m.brand(ctx, uid, id, true, func(tx ManagementTx, brand *entities.Brand) error {
		if brand.Status() == entities.BrandSuspended {
			return ErrForbidden
		}
		saved, err := tx.BrandPage(ctx)
		if err != nil {
			return err
		}
		if saved == nil {
			page.Draft = entities.DefaultBrandPage(brand.Appearance())
		} else {
			page = *saved
		}
		if change != nil {
			if err := change(&page); err != nil {
				return err
			}
			if err := tx.SaveBrandPage(ctx, page); err != nil {
				return err
			}
		}
		page.MediaIDs, err = tx.BrandEditorialMedia(ctx)
		return err
	})
	return page, err
}
