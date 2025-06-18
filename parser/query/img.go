package query

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/chai2010/webp"
)

func ConvertToWebp(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	err := webp.Encode(&buf, img, &webp.Options{Lossless: false, Quality: 75})
	if err != nil {
		return nil, fmt.Errorf("webp encode failed: %w", err)
	}
	return buf.Bytes(), nil
}

func filterImg(resp *http.Response, url string) ([]byte, string, error) {
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		ext := filepath.Ext(url)
		contentType = mime.TypeByExtension(ext)
	}
	slog.Info("Processing image", "contentType", contentType, "url", url)

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, contentType, fmt.Errorf("read body: %w", err)
	}
	if len(imgBytes) == 0 {
		return nil, contentType, fmt.Errorf("empty image data")
	}

	switch contentType {
	case "image/webp":
		return imgBytes, "image/webp", nil

	case "image/jpeg", "image/jpg", "image/png", "image/gif":
		img, _, err := image.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return nil, contentType, fmt.Errorf("image decode failed: %w", err)
		}
		imgBytes, err := ConvertToWebp(img)
		if err != nil {
			return nil, "image/webp", fmt.Errorf("convert to webp failed: %w", err)
		}
		return imgBytes, "image/webp", nil

	default:
		return nil, "", fmt.Errorf("unsupported image format: %s", contentType)
	}
}
