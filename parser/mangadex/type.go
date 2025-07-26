package mangadex

// import (
// 	"time"

// 	"github.com/google/uuid"
// )

// // ///////////////////manga json
// type MangaListReq struct {
// 	Result   string      `json:"result"`
// 	Response string      `json:"response"`
// 	Data     []MangaData `json:"data"`
// 	Limit    int         `json:"limit"`
// 	Offset   int         `json:"offset"`
// 	Total    int         `json:"total"`
// }

// type MangaData struct {
// 	ID            string          `json:"id"`
// 	Type          string          `json:"type"`
// 	Attributes    MangaAttributes `json:"attributes"`
// 	Relationships []Relationship  `json:"relationships"`
// }

// type MangaAttributes struct {
// 	Title                          map[string]string   `json:"title"`
// 	AltTitles                      []map[string]string `json:"altTitles"`
// 	Description                    map[string]string   `json:"description"`
// 	IsLocked                       bool                `json:"isLocked"`
// 	Links                          map[string]string   `json:"links"`
// 	OriginalLanguage               string              `json:"originalLanguage"`
// 	LastVolume                     string              `json:"lastVolume"`
// 	LastChapter                    string              `json:"lastChapter"`
// 	PublicationDemographic         string              `json:"publicationDemographic"`
// 	Status                         string              `json:"status"`
// 	Year                           int                 `json:"year"`
// 	ContentRating                  string              `json:"contentRating"`
// 	Tags                           []Tag               `json:"tags"`
// 	State                          string              `json:"state"`
// 	ChapterNumbersResetOnNewVolume bool                `json:"chapterNumbersResetOnNewVolume"`
// 	CreatedAt                      string              `json:"createdAt"`
// 	UpdatedAt                      string              `json:"updatedAt"`
// 	Version                        int                 `json:"version"`
// 	AvailableTranslatedLanguages   []string            `json:"availableTranslatedLanguages"`
// 	LatestUploadedChapter          string              `json:"latestUploadedChapter"`
// }

// type Tag struct {
// 	ID            string         `json:"id"`
// 	Type          string         `json:"type"`
// 	Attributes    TagAttributes  `json:"attributes"`
// 	Relationships []Relationship `json:"relationships"`
// }

// type TagAttributes struct {
// 	Name        map[string]string `json:"name"`
// 	Description map[string]string `json:"description"`
// 	Group       string            `json:"group"`
// 	Version     int               `json:"version"`
// }

// type Relationship struct {
// 	ID         string         `json:"id"`
// 	Type       string         `json:"type"`
// 	Related    string         `json:"related,omitempty"`
// 	Attributes *RelAttributes `json:"attributes,omitempty"`
// }

// type RelAttributes struct {
// 	Description string `json:"description"`
// 	Volume      string `json:"volume"`
// 	FileName    string `json:"fileName"`
// 	Locale      string `json:"locale"`
// 	CreatedAt   string `json:"createdAt"`
// 	UpdatedAt   string `json:"updatedAt"`
// 	Version     int    `json:"version"`
// }

// /////////////////////manga json

// type ChapterListResp struct {
// 	Result  string                `json:"result"`
// 	Volumes map[string]VolumeInfo `json:"volumes"`
// }
// type VolumeInfo struct {
// 	Chapters map[string]ChapterInfo `json:"chapters"`
// }
// type ChapterInfo struct {
// 	Chapter string `json:"chapter"`
// 	ID      string `json:"id"`
// }

// // ////////////////////////////////////
// type ChapterImgListResp struct {
// 	BaseURL string     `json:"baseUrl"`
// 	Chapter ChapterImg `json:"chapter"`
// }
// type ChapterImg struct {
// 	Hash      string   `json:"hash"`
// 	Data      []string `json:"data"`
// 	DataSaver []string `json:"dataSaver"`
// }

// // ////////////////////////////////////
// // type MangaData struct {
// // 	ID         string `json:"id"`
// // 	Type       string `json:"type"`
// // 	Attributes struct {
// // 		Title struct {
// // 			En string `json:"en"`
// // 		} `json:"title"`
// // 		AltTitles []struct {
// // 			Ja string `json:"ja"`
// // 			En string `json:"en,omitempty"`
// // 		} `json:"altTitles"`
// // 		Description struct {
// // 			En string `json:"en"`
// // 		} `json:"description"`
// // 		IsLocked bool `json:"isLocked"`
// // 		Links    struct {
// // 			Mal string `json:"mal"`
// // 		} `json:"links"`
// // 		OriginalLanguage       string      `json:"originalLanguage"`
// // 		LastChapter            string      `json:"lastChapter"`
// // 		Status                 string      `json:"status"`
// // 		PublicationDemographic interface{} `json:"publicationDemographic"`
// // 		Year                   int         `json:"year"`
// // 		ContentRating          string      `json:"contentRating"`
// // 		Tags                   []struct {
// // 			Attributes struct {
// // 				Name struct {
// // 					En string `json:"en"`
// // 				} `json:"name"`
// // 			} `json:"attributes"`
// // 		} `json:"tags"`
// // 		State                 string      `json:"state"`
// // 		CreatedAt             time.Time   `json:"createdAt"`
// // 		UpdatedAt             time.Time   `json:"updatedAt"`
// // 		LatestUploadedChapter interface{} `json:"latestUploadedChapter"`
// // 	} `json:"attributes"`
// // }

// type ChapterDB struct {
// 	ID        uuid.UUID `db:"id"`
// 	MangaID   uuid.UUID `db:"manga_id"`
// 	Title     string    `db:"title"`
// 	UpdatedAt time.Time `db:"updated_at"`
// 	CreatedAt time.Time `db:"created_at"`
// 	Number    int       `db:"number"`
// 	Imgs      []string  `db:"imgs"`
// }

// type MangaDB struct {
// 	ID                     uuid.UUID `db:"id"`
// 	Type                   string    `db:"type"`
// 	TitleEn                *string   `db:"title_en"`
// 	AltTitles              []string  `db:"alt_titles"`
// 	DescriptionEn          *string   `db:"description_en"`
// 	LatestUploadedChapter  *int      `db:"latest_uploaded_chapter"`
// 	Tags                   []string  `db:"tags"`
// 	OriginalLanguage       *string   `db:"original_language"`
// 	LastChapter            *string   `db:"last_chapter"`
// 	Status                 *string   `db:"status"`
// 	PublicationDemographic *string   `db:"publication_demographic"`
// 	Year                   *int      `db:"year"`
// 	ContentRating          *string   `db:"content_rating"`
// 	State                  *string   `db:"state"`
// 	CreatedAt              time.Time `db:"created_at"`
// 	UpdatedAt              time.Time `db:"updated_at"`
// 	IsLocked               bool      `db:"is_locked"`
// 	LinksMal               *string   `db:"links_mal"`
// }
