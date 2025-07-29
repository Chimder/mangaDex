package query

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/prometheus/client_golang/prometheus"
)

func SafeChapterNameToS3(name string) string {
	name = strings.TrimSpace(name)

	reg := regexp.MustCompile(`[^\w\s.-]`)
	name = reg.ReplaceAllString(name, "_")

	name = strings.ReplaceAll(name, " ", "_")

	return url.PathEscape(name)
}

func ConvertToWebp(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	err := webp.Encode(&buf, img, &webp.Options{Lossless: false, Quality: 75})
	if err != nil {
		return nil, fmt.Errorf("webp encode failed: %w", err)
	}
	return buf.Bytes(), nil
}
func extractExtAndMime(urlStr string) (string, string) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", ""
	}
	ext := filepath.Ext(u.Path)

	var contentType string
	if ext != "" {
		contentType = mime.TypeByExtension(ext)
	}

	return ext, contentType
}

func FilterImg(resp *http.Response, url string, duration *prometheus.HistogramVec, start time.Time) ([]byte, string, string, error) {
	defer resp.Body.Close()
	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", "", fmt.Errorf("read body: %w", err)
	}
	if len(imgBytes) == 0 {
		return nil, "", "", fmt.Errorf("empty image data")
	}

	duration.WithLabelValues("img").Observe(time.Since(start).Seconds())
	ext, contentType := extractExtAndMime(url)

	switch contentType {
	case "image/webp":
		return imgBytes, ".webp", "image/webp", nil

	case "image/jpeg", "image/jpg", "image/png", "image/gif":
		img, _, err := image.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return imgBytes, ext, contentType, nil
		}
		webpBytes, err := ConvertToWebp(img)
		if err != nil {
			return imgBytes, ext, contentType, nil
		}
		return webpBytes, ".webp", "image/webp", nil

	default:
		return imgBytes, ext, contentType, nil
	}
}
