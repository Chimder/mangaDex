package mangadex

import (
	"context"
	"encoding/json"
	"fmt"
	"mangadex/parser/proxy"
	"net/http"
	"net/url"
	"strconv"
)

type Query struct{}

func NewQueryManager() *Query {
	return &Query{}
}

func (qm *Query) GetMangaList(ctx context.Context, limit, offset int, c *proxy.ProxyClient) (*MangaListReq, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
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

	return allChapters[0:10], nil
}

type ChapterImgsData struct {
	chapterId string
	imgLinks  []string
}

func (qm *Query) GetChapterimgsById(ctx context.Context, chapterID string, c *proxy.ProxyClient) (*ChapterImgsData, error) {
	urlStr := fmt.Sprintf("https://api.mangadex.org/at-home/server/%s?forcePort443=false", chapterID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
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

func (qm *Query) GetMangaImgs() {}
