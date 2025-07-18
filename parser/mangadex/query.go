package mangadex

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mangadex/parser/proxy"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Query struct{}

func NewQueryManager() *Query {
	return &Query{}
}

func (qm *Query) GetMangaList(ctx context.Context, limit, offset int, c *proxy.ProxyClient) (*MangaListReq, error) {
	slog.Info("GEtManagList", "off", offset)
	u, _ := url.Parse("https://api.mangadex.org/manga")
	params := url.Values{
		"limit":                         []string{strconv.Itoa(limit)},
		"offset":                        []string{strconv.Itoa(offset)},
		"includes[]":                    []string{"cover_art"},
		"contentRating[]":               []string{"safe", "suggestive", "erotica"},
		"availableTranslatedLanguage[]": []string{"en"},
		"order[rating]":                 []string{"desc"},
		"includedTagsMode":              []string{"AND"},
		"excludedTagsMode":              []string{"OR"},
	}
	u.RawQuery = params.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := c.CreateMangaRequest(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result MangaListReq
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

type ChapterList struct {
	ID            string
	ChapterNumber string
}

func (qm *Query) GetChapterListById(ctx context.Context, mangaID string, c *proxy.ProxyClient) ([]ChapterList, error) {
	urlStr := fmt.Sprintf("https://api.mangadex.org/manga/%s/aggregate?translatedLanguage[]=en", mangaID)

	reqCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	req, err := c.CreateMangaRequest(reqCtx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data ChapterListResp
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return nil, err
	}

	var allChapters []ChapterList

	for _, volume := range data.Volumes {
		for _, chapter := range volume.Chapters {
			allChapters = append(allChapters, ChapterList{
				ChapterNumber: chapter.Chapter,
				ID:            chapter.ID,
			})
		}
	}

	if len(allChapters) > 1 {
		return allChapters[0:1], nil
	}
	return allChapters, nil
}

type ChapterImgsData struct {
	chapterId string
	imgLinks  []string
}

func (qm *Query) GetChapterImgsById(ctx context.Context, chapterID string, c *proxy.ProxyClient) (*ChapterImgsData, error) {
	urlStr := fmt.Sprintf("https://api.mangadex.org/at-home/server/%s?forcePort443=false", chapterID)

	reqCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	req, err := c.CreateMangaRequest(reqCtx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data ChapterImgListResp
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return nil, err
	}

	quality := "data"
	pageFiles := data.Chapter.Data
	if len(pageFiles) == 0 {
		quality = "data-saver"
		pageFiles = data.Chapter.DataSaver
	}

	result := make([]string, len(pageFiles))
	for i, v := range pageFiles {
		fullURL := fmt.Sprintf("%s/%s/%s/%s", data.BaseURL, quality, data.Chapter.Hash, v)
		result[i] = fullURL

	}
	return &ChapterImgsData{chapterID, result}, nil
}

func (qm *Query) DownloadImage(ctx context.Context, imageURL string, c *proxy.ProxyClient) (*http.Response, error) {
	reqCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create image request: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://mangadex.org/")
	req.Header.Set("Accept", "image/webp,image/apng,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "close")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image download failed: %v", err)
	}

	return resp, err
}
func (qm *Query) GetMangaImgs() {}
