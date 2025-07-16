package mangadex

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MangaRepository interface {
	ExistsMangaByTitle(ctx context.Context, title string) (string, error)
	InsertManga(ctx context.Context, arg *MangaData) (string, error)
	GetMangaById(ctx context.Context, id string) (*MangaDB, error)
	InsertChapter(ctx context.Context, arg *InsertChapterArgs) (string, error)
	GetChapterById(ctx context.Context, id string) (*ChapterDB, error)
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

func (r *mangaRepository) GetMangaById(ctx context.Context, id string) (*MangaDB, error) {
	query := `SELECT * FROM manga WHERE id = $1`
	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return &MangaDB{}, fmt.Errorf("err fetch user stats  %w", err)
	}

	return pgx.CollectOneRow(rows, pgx.RowToStructByName[*MangaDB])
}

func (q *mangaRepository) InsertManga(ctx context.Context, arg *MangaData) (string, error) {
	query := `
    INSERT INTO manga (
        title_en, alt_titles, description_en,
        tags, original_language, last_chapter,
        status, publication_demographic, year,
        content_rating, state,
        is_locked, links_mal
    ) VALUES (
        @title_en, @alt_titles, @description_en,
        @tags, @original_language, @last_chapter,
        @status, @publication_demographic, @year,
        @content_rating, @state,
        @is_locked, @links_mal
    )
    RETURNING id;
    `

	var altTitles []string
	for _, v := range arg.Attributes.AltTitles {
		if val, ok := v["en"]; ok && val != "" {
			altTitles = append(altTitles, val)
		} else if val, ok := v["ja"]; ok && val != "" {
			altTitles = append(altTitles, val)
		}
	}

	var tags []string
	for _, v := range arg.Attributes.Tags {
		if val, ok := v.Attributes.Name["en"]; ok && val != "" {
			tags = append(tags, val)
		}
	}

	var id string
	err := q.db.QueryRow(ctx, query, pgx.NamedArgs{
		"title_en":                arg.Attributes.Title["en"],
		"alt_titles":              altTitles,
		"description_en":          arg.Attributes.Description["en"],
		"tags":                    tags,
		"original_language":       arg.Attributes.OriginalLanguage,
		"last_chapter":            arg.Attributes.LastChapter,
		"status":                  arg.Attributes.Status,
		"publication_demographic": arg.Attributes.PublicationDemographic,
		"year":                    arg.Attributes.Year,
		"content_rating":          arg.Attributes.ContentRating,
		"state":                   arg.Attributes.State,
		"is_locked":               arg.Attributes.IsLocked,
		"links_mal":               arg.Attributes.Links["mal"],
	}).Scan(&id)
	if err != nil {
		return id, err
	}

	return id, err
}

////////

type InsertChapterArgs struct {
	manga_id uuid.UUID
	title    string
	number   int
	imgs     []string
}

func (q *mangaRepository) InsertChapter(ctx context.Context, arg *InsertChapterArgs) (string, error) {
	query := `
    INSERT INTO chapter (manga_id, title, number, imgs) VALUES (
        @manga_id, @title, @number, @imgs)
    RETURNING id;
    `

	var id string
	err := q.db.QueryRow(ctx, query, pgx.NamedArgs{
		"manga_id": arg.manga_id,
		"title":    arg.title,
		"number":   arg.number,
		"img":      arg.imgs,
	}).Scan(&id)
	if err != nil {
		return id, err
	}

	return id, err
}

func (r *mangaRepository) GetChapterById(ctx context.Context, id string) (*ChapterDB, error) {
	query := `SELECT * FROM chapter WHERE id = $1`
	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("err fetch user stats  %w", err)
	}

	return pgx.CollectOneRow(rows, pgx.RowToStructByName[*ChapterDB])
}

func (r *mangaRepository) GetChaptersByMangaId(ctx context.Context, id string) ([]ChapterDB, error) {
	query := `SELECT * FROM chapter WHERE manga_id = $1 ORDER BY number DESC`
	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("err fetch user stats  %w", err)
	}

	chapters, err := pgx.CollectRows(rows, pgx.RowToStructByName[ChapterDB])
	if err != nil {
		return nil, fmt.Errorf("failed to collect chapter rows: %w", err)
	}

	return chapters, nil
}
