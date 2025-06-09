package query

import "time"

type MangaInfoParserResp struct {
	Title       string
	CoverURL    string
	AltTitles   []string
	Status      string
	Authors     []string
	Description string
	Genres      []string
	Chapters    []string
}

type AllPopularMangaResp struct {
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
				En string `json:"en,omitempty"`
			} `json:"altTitles"`
			Description struct {
				En string `json:"en"`
			} `json:"description"`
			IsLocked bool `json:"isLocked"`
			Links    struct {
				Mal string `json:"mal"`
			} `json:"links"`
			LastVolume             string      `json:"lastVolume"`
			LastChapter            string      `json:"lastChapter"`
			PublicationDemographic interface{} `json:"publicationDemographic"`
			Status                 string      `json:"status"`
			Year                   int         `json:"year"`
			ContentRating          string      `json:"contentRating"`
			Tags                   []struct {
				Attributes struct {
					Name struct {
						En string `json:"en"`
					} `json:"attributes"`
				} `json:"tags"`
				State     string    `json:"state"`
				CreatedAt time.Time `json:"createdAt"`
				UpdatedAt time.Time `json:"updatedAt"`
				Version   int       `json:"version"`
			} `json:"attributes"`
		}
	} `json:"data"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}
