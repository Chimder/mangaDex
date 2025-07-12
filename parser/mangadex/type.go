package mangadex

import "time"

type ChapterInfo struct {
	Chapter string `json:"chapter"`
	ID      string `json:"id"`
}

type VolumeInfo struct {
	Chapters map[string]ChapterInfo `json:"chapters"`
}

type ChapterListResp struct {
	Result  string                `json:"result"`
	Volumes map[string]VolumeInfo `json:"volumes"`
}

type ChapterImgListResp struct {
	BaseURL string      `json:"baseUrl"`
	Chapter ChapterImg `json:"chapter"`
}
type ChapterImg struct {
	Hash      string   `json:"hash"`
	Data      []string `json:"data"`
	DataSaver []string `json:"dataSaver"`
}

type MangaListReq struct {
	Result   string `json:"result"`
	Response string `json:"response"`
	Data     []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			Title struct {
				En string `json:"en"`
			} `json:"title"`
			AltTitles []struct {
				Ja string `json:"ja"`
				En string `json:"en,omitempty"`
			} `json:"altTitles"`
			Description struct {
				En string `json:"en"`
			} `json:"description"`
			IsLocked bool `json:"isLocked"`
			Links    struct {
				Mal string `json:"mal"`
			} `json:"links"`
			OriginalLanguage       string      `json:"originalLanguage"`
			LastChapter            string      `json:"lastChapter"`
			Status                 string      `json:"status"`
			PublicationDemographic interface{} `json:"publicationDemographic"`
			Year                   int         `json:"year"`
			ContentRating          string      `json:"contentRating"`
			Tags                   []struct {
				Attributes struct {
					Name struct {
						En string `json:"en"`
					} `json:"name"`
				} `json:"attributes"`
			} `json:"tags"`
			State                 string      `json:"state"`
			CreatedAt             time.Time   `json:"createdAt"`
			UpdatedAt             time.Time   `json:"updatedAt"`
			LatestUploadedChapter interface{} `json:"latestUploadedChapter"`
		} `json:"attributes"`
	} `json:"data"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}
