package mangapark

import "time"

type MangaInfoParserResp struct {
	Title       string
	CoverURL    string
	AltTitles   []string
	Status      string
	Authors     []string
	Description string
	Genres      []string
	Chapters    []Chapter
}
type Chapter struct {
	Name string `json:"name"`
	URL  string `json:"url"`
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

type ImgInfoToPool struct {
	Url         string `json:"url"`
	Idx         int    `json:"idx"`
	MangaID     string `json:"manga_id"`
	ChapterName string `json:"chapter_name"`
}

type ChapterInfoToPool struct{
	Url         string `json:"url"`
	MangaID     string `json:"manga_id"`
	// Idx         int    `json:"idx"`
	// ChapterName string `json:"chapter_name"`
}