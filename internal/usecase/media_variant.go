package usecase

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strconv"

	"golang.org/x/image/draw"
	"srez/internal/entities"
)

var (
	ErrMediaObjectMissing      = errors.New("object storage operation failed (HTTP 404)")
	ErrMediaStorageUnavailable = errors.New("object storage temporarily unavailable")
)

var variantWidths = [...]int{480, 960, 1440}

func ValidMediaWidth(width int) bool {
	for _, size := range variantWidths {
		if size == width {
			return true
		}
	}
	return false
}

type readyMediaLock interface {
	WithReadyMedia(context.Context, int64, func() error) error
}

func variantKey(key string, width int) string {
	return key + ".w" + strconv.Itoa(width) + "-v1"
}

func hasMediaVariants(purpose string) bool {
	return purpose == "brand_banner" || purpose == EditorialMediaPurpose
}

func (m *Media) readVariant(ctx context.Context, asset *MediaAsset, width int) ([]byte, error) {
	key := variantKey(asset.ObjectKey, width)
	data, err := m.readObject(ctx, key)
	if !errors.Is(err, ErrMediaObjectMissing) {
		return data, err
	}
	locker, ok := m.repo.(readyMediaLock)
	if !ok {
		return nil, ErrConflict
	}
	// GC locks the same asset before retiring it. No object can be created after
	// deletion wins. The second read also deduplicates first requests across processes.
	err = locker.WithReadyMedia(ctx, asset.ID, func() error {
		data, err = m.readObject(ctx, key)
		if !errors.Is(err, ErrMediaObjectMissing) {
			return err
		}
		original, err := m.readObject(ctx, asset.ObjectKey)
		if err != nil {
			return err
		}
		if int64(len(original)) != asset.Size {
			return ErrConflict
		}
		data, err = resizeMedia(original, asset.MIME, width)
		if err != nil {
			return err
		}
		return m.objects.Put(ctx, key, asset.MIME, bytes.NewReader(data))
	})
	return data, err
}

func (m *Media) readObject(ctx context.Context, key string) ([]byte, error) {
	r, err := m.objects.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, MaxMediaOutputBytes+1))
	if err != nil {
		return nil, ErrMediaStorageUnavailable
	}
	if len(data) == 0 || int64(len(data)) > MaxMediaOutputBytes {
		return nil, ErrConflict
	}
	return data, nil
}

func resizeMedia(data []byte, mime string, width int) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 ||
		int64(cfg.Width)*int64(cfg.Height) > MaxMediaPixels {
		return nil, ErrMediaCorrupt
	}
	if cfg.Width <= width {
		return data, nil
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrMediaCorrupt
	}
	height := max(1, cfg.Height*width/cfg.Width)
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	var out boundedImageBuffer
	switch mime {
	case "image/jpeg":
		err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 82})
	case "image/png":
		err = png.Encode(&out, dst)
	default:
		return nil, entities.ErrInvalidValue
	}
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
