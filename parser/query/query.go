package query

import (
	"encoding/json"
	"fmt"
	"mangadex/parser/proxy"
	"net/http"
	"net/url"
)

var baseURL = "https://api.mangadex.org/manga"

func GetMngaAggregate(proxy *proxy.ProxyClient, id string, offset int) (*AllPopularMangaResp, error) {
	params := url.Values{}
	params.Add("translatedLanguage[]", "en")

	fullURL := fmt.Sprintf("%s/%s?%s", baseURL, id+"/aggregate", params.Encode())
	resp, err := proxy.Client.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manga aggregate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var respData AllPopularMangaResp
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &respData, nil
}
func GetMangaAggregate(proxy *proxy.ProxyClient, id string, offset int) (*AllPopularMangaResp, error) {
	params := url.Values{}
	params.Add("translatedLanguage[]", "en")

	fullURL := fmt.Sprintf("%s/%s?%s", baseURL, id+"/aggregate", params.Encode())
	resp, err := proxy.Client.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manga aggregate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var respData AllPopularMangaResp
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &respData, nil
}

func GetAllPopularManga(proxy *proxy.ProxyClient, limit, offset int) (*AllPopularMangaResp, error) {
	params := url.Values{}
	params.Add("limit", fmt.Sprintf("%d", limit))
	params.Add("offset", fmt.Sprintf("%d", offset))
	params.Add("contentRating[]", "safe")
	params.Add("contentRating[]", "suggestive")
	params.Add("contentRating[]", "erotica")
	params.Add("order[followedCount]", "desc")
	params.Add("includedTagsMode", "AND")
	params.Add("excludedTagsMode", "OR")

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	resp, err := proxy.Client.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch mangas: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var respData AllPopularMangaResp
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &respData, nil
}
