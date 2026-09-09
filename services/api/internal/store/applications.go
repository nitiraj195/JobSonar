package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ValidStatuses = []string{"saved", "applied", "screen", "interview", "offer", "closed"}

func ValidStatus(s string) bool {
	for _, v := range ValidStatuses {
		if v == s {
			return true
		}
	}
	return false
}

func (s *Store) ListApplications(ctx context.Context, profileID uuid.UUID) ([]Application, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.job_id, j.title, j.company, j.location, j.source_url,
		       a.resume_variant, a.status, a.applied_at, a.notes, a.created_at
		FROM applications a
		JOIN jobs j ON j.id = a.job_id
		WHERE a.profile_id = $1
		ORDER BY a.created_at DESC
	`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Application
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if out == nil {
		out = []Application{}
	}
	return out, rows.Err()
}

func (s *Store) CreateApplication(ctx context.Context, profileID, jobID uuid.UUID) (Application, error) {
	if _, err := s.GetJob(ctx, profileID, jobID); err != nil {
		return Application{}, err
	}
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO applications (job_id, profile_id, status)
		VALUES ($1, $2, 'saved')
		ON CONFLICT (job_id, profile_id) DO UPDATE SET job_id = EXCLUDED.job_id
		RETURNING id
	`, jobID, profileID).Scan(&id)
	if err != nil {
		return Application{}, err
	}
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO application_events (application_id, from_status, to_status)
		SELECT $1, NULL, 'saved'
		WHERE NOT EXISTS (
			SELECT 1 FROM application_events WHERE application_id = $1
		)
	`, id)
	return s.getApplication(ctx, id)
}

func (s *Store) UpdateApplicationStatus(ctx context.Context, id uuid.UUID, status string) (Application, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Application{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var from string
	err = tx.QueryRow(ctx, `SELECT status FROM applications WHERE id = $1`, id).Scan(&from)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	if err != nil {
		return Application{}, err
	}
	if from == status {
		if err := tx.Commit(ctx); err != nil {
			return Application{}, err
		}
		return s.getApplication(ctx, id)
	}

	var appliedAt *time.Time
	if status == "applied" {
		t := time.Now().UTC()
		appliedAt = &t
	}
	_, err = tx.Exec(ctx, `
		UPDATE applications
		SET status = $2,
		    applied_at = COALESCE($3, applied_at)
		WHERE id = $1
	`, id, status, appliedAt)
	if err != nil {
		return Application{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO application_events (application_id, from_status, to_status)
		VALUES ($1, $2, $3)
	`, id, from, status)
	if err != nil {
		return Application{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Application{}, err
	}
	return s.getApplication(ctx, id)
}

func (s *Store) getApplication(ctx context.Context, id uuid.UUID) (Application, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT a.id, a.job_id, j.title, j.company, j.location, j.source_url,
		       a.resume_variant, a.status, a.applied_at, a.notes, a.created_at
		FROM applications a
		JOIN jobs j ON j.id = a.job_id
		WHERE a.id = $1
	`, id)
	a, err := scanApplication(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	return a, err
}

func scanApplication(row rowScanner) (Application, error) {
	var a Application
	if err := row.Scan(&a.ID, &a.JobID, &a.Title, &a.Company, &a.Location, &a.SourceURL,
		&a.ResumeVariant, &a.Status, &a.AppliedAt, &a.Notes, &a.CreatedAt); err != nil {
		return Application{}, err
	}
	return a, nil
}
