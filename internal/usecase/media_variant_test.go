package usecase

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestResizeMediaPreservesRatioAndTransparency(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 1000, 600))
	source.SetNRGBA(500, 300, color.NRGBA{R: 246, A: 100})
	for _, mime := range []string{"image/jpeg", "image/png"} {
		var data bytes.Buffer
		var err error
		if mime == "image/jpeg" {
			err = jpeg.Encode(&data, source, nil)
		} else {
			err = png.Encode(&data, source)
		}
		if err != nil {
			t.Fatal(err)
		}
		resized, err := resizeMedia(data.Bytes(), mime, 480)
		if err != nil {
			t.Fatal(err)
		}
		decoded, _, err := image.Decode(bytes.NewReader(resized))
		if err != nil || decoded.Bounds().Dx() != 480 || decoded.Bounds().Dy() != 288 {
			t.Fatal(err)
		}
		if _, _, _, alpha := decoded.At(0, 0).RGBA(); mime == "image/png" && alpha != 0 {
			t.Fatal("alpha lost")
		}
		small, err := resizeMedia(data.Bytes(), mime, 1440)
		if err != nil || !bytes.Equal(small, data.Bytes()) {
			t.Fatal("small original altered", err)
		}
	}
}
