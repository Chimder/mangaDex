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
	"strings"

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

func normalizeExt(ext string) string {
	ext = strings.ToLower(ext)
	if ext == "" {
		return ".bin"
	}
	if !strings.HasPrefix(ext, ".") {
		return "." + ext
	}
	return ext
}

func FilterImg(resp *http.Response, url string) ([]byte, string, error) {
	imgBytes, err := io.ReadAll(resp.Body)
	// log.Printf("Start Filter",":",url)
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}
	if len(imgBytes) == 0 {
		return nil, "", fmt.Errorf("empty image data")
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(url))
	}
	ext := normalizeExt(filepath.Ext(url))

	switch contentType {
	case "image/webp":
		return imgBytes, ".webp", nil

	case "image/jpeg", "image/jpg", "image/png", "image/gif":
		img, _, err := image.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			slog.Warn("image decode failed", "err", err)
			return imgBytes, ext, nil
		}
		webpBytes, err := ConvertToWebp(img)
		if err != nil {
			slog.Warn("webp convert failed", "err", err)
			return imgBytes, ext, nil
		}
		return webpBytes, ".webp", nil

	default:
		return imgBytes, ext, nil
	}
}
