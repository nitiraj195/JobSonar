package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateResume(ctx context.Context, profileID uuid.UUID, storageURI string) (Resume, error) {
	var r Resume
	err := s.pool.QueryRow(ctx, `
		INSERT INTO resumes (profile_id, storage_uri, status)
		VALUES ($1, $2, 'pending')
		RETURNING id, status, error, created_at
	`, profileID, storageURI).Scan(&r.ID, &r.Status, &r.Error, &r.CreatedAt)
	return r, err
}

func (s *Store) LatestResume(ctx context.Context, profileID uuid.UUID) (Resume, error) {
	var r Resume
	err := s.pool.QueryRow(ctx, `
		SELECT id, status, error, created_at
		FROM resumes
		WHERE profile_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, profileID).Scan(&r.ID, &r.Status, &r.Error, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Resume{}, ErrNotFound
	}
	return r, err
}
