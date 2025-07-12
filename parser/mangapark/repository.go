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
		return MangaChaptersInfo{}, fmt.Errorf("err fetch user stats  %w", err)
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
