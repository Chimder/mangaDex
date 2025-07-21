package mangapark

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MangaDB struct {
	Id             uuid.UUID `db:"id"`
	Title          string    `db:"title"`
	CoverUrl       string    `db:"cover_url"`
	AltTitles      []string  `db:"alt_titles"`
	Status         string    `db:"status"`
	Authors        []string  `db:"authors"`
	Description    string    `db:"description"`
	Genres         []string  `db:"genres"`
	LastChapter    float64   `db:"last_chapter"`
	ChaptersAmount int       `db:"chapters_amount"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type MangaRepository interface {
	InsertManga(ctx context.Context, arg MangaDB) (string, error)
	GetMangaChaptersById(ctx context.Context, id string) (MangaChaptersInfo, error)
	ExistsMangaByTitle(ctx context.Context, title string) (string, error)

	GetChapters(ctx context.Context) ([]ChapterDB, error)
	GetChaptersNamesByMangaId(ctx context.Context, id string) ([]ChapterNamesDB, error)
	CreateChapter(ctx context.Context, ch CreateChapterArg) (bool, error)

	GetImgTasks(ctx context.Context) ([]ImgInfoToChan, error)
	CreateImgTask(ctx context.Context, img ImgInfoToChan) (bool, error)
	DeleteImgTaskByURL(ctx context.Context, url string) (bool, error)
}

type mangaRepository struct {
	db *pgxpool.Pool
}

func NewMangaRepository(db *pgxpool.Pool) MangaRepository {
	return &mangaRepository{
		db: db,
	}
}

type MangaChaptersInfo struct {
	ID             uuid.UUID `db:"id"`
	Title          string    `db:"title"`
	MangaID        uuid.UUID `db:"manga_id"`
	LastChapter    float64   `db:"last_chapter"`
	ChaptersAmount int       `db:"chapters_amount"`
}

func (r *mangaRepository) ExistsMangaByTitle(ctx context.Context, title string) (string, error) {
	query := `SELECT id FROM manga WHERE title = $1 LIMIT 1`

	var mangaId uuid.UUID
	err := r.db.QueryRow(ctx, query, title).Scan(&mangaId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		return "", err
	}
	return mangaId.String(), nil
}

func (r *mangaRepository) GetMangaChaptersById(ctx context.Context, id string) (MangaChaptersInfo, error) {
	query := `SELECT (last_chapter, chapters_amount, manga_id) FROM manga WHERE id = $1`
	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return MangaChaptersInfo{}, fmt.Errorf("err fetch manga %w", err)
	}
	defer rows.Close()

	return pgx.CollectOneRow(rows, pgx.RowToStructByName[MangaChaptersInfo])
}

func (q *mangaRepository) InsertManga(ctx context.Context, arg MangaDB) (string, error) {
	query := `
		INSERT INTO "manga" (
			title, cover_url, alt_titles, status,
			authors, description, genres
		)
		VALUES (
			@title, @cover_url, @alt_titles, @status,
			@authors, @description, @genres
		)
		RETURNING id
	`

	var id string
	err := q.db.QueryRow(ctx, query, pgx.NamedArgs{
		"title":       arg.Title,
		"cover_url":   arg.CoverUrl,
		"alt_titles":  arg.AltTitles,
		"status":      arg.Status,
		"authors":     arg.Authors,
		"description": arg.Description,
		"genres":      arg.Genres,
	}).Scan(&id)
	if err != nil {
		return id, err
	}

	return id, err
}

type ChapterDB struct {
	Id        uuid.UUID
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	MangaId   string    `json:"manga_id"`
	Name      string    `json:"name"`
	Number    int       `json:"number"`
	Imgs      []string  `json:"img"`
}

func (r *mangaRepository) GetChapters(ctx context.Context) ([]ChapterDB, error) {
	query := `SELECT * FROM chapter`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("err fetch all chapter %w", err)
	}

	return pgx.CollectRows(rows, pgx.RowToStructByName[ChapterDB])
}

type ChapterNamesDB struct {
	Name string `json:"name"`
}

func (r *mangaRepository) GetChaptersNamesByMangaId(ctx context.Context, id string) ([]ChapterNamesDB, error) {
	query := `SELECT name FROM chapter WHERE manga_id = $1`

	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("err fetch all chapter %w", err)
	}

	return pgx.CollectRows(rows, pgx.RowToStructByName[ChapterNamesDB])
}

type CreateChapterArg struct {
	manga_id string
	name     string
	imgs     []string
}

func (r *mangaRepository) CreateChapter(ctx context.Context, ch CreateChapterArg) (bool, error) {
	query := `INSERT INTO chapter (manga_id, name, imgs)
	          VALUES (@manga_id, @name, @imgs)`

	res, err := r.db.Exec(ctx, query, pgx.NamedArgs{
		"manga_id": ch.manga_id,
		"name":     ch.name,
		"imgs":     ch.imgs,
	})
	if err != nil {
		return false, fmt.Errorf("err create chapter: %w", err)
	}

	return res.RowsAffected() > 0, nil
}
func (r *mangaRepository) GetImgTasks(ctx context.Context) ([]ImgInfoToChan, error) {
	query := `SELECT * FROM img_task`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("err fetch img_task  %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, pgx.RowToStructByName[ImgInfoToChan])
}

func (r *mangaRepository) CreateImgTask(ctx context.Context, img ImgInfoToChan) (bool, error) {
	query := `INSERT INTO img_task (url, idx, manga_id, chapter_name)
	          VALUES (@url, @idx, @manga_id, @chapter_name)`

	res, err := r.db.Exec(ctx, query, pgx.NamedArgs{
		"url":          img.Url,
		"idx":          img.Idx,
		"manga_id":     img.MangaId,
		"chapter_name": img.ChapterName,
	})
	if err != nil {
		return false, fmt.Errorf("err create img_task: %w", err)
	}

	return res.RowsAffected() > 0, nil
}

func (r *mangaRepository) DeleteImgTaskByURL(ctx context.Context, url string) (bool, error) {
	query := `DELETE FROM img_task WHERE url = $1`

	res, err := r.db.Exec(ctx, query, url)
	if err != nil {
		return false, err
	}

	return res.RowsAffected() > 0, nil
}
