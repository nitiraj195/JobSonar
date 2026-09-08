package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type TailorJob struct {
	ID            uuid.UUID  `json:"id"`
	JobID         *uuid.UUID `json:"job_id,omitempty"`
	Source        string     `json:"source"`
	Title         string     `json:"title"`
	Company       string     `json:"company"`
	JDMD          string     `json:"jd_md,omitempty"`
	ResumeMD      string     `json:"resume_md,omitempty"`
	CoverLetterMD string     `json:"cover_letter_md,omitempty"`
	NotesMD       string     `json:"notes_md,omitempty"`
	Model         string     `json:"model,omitempty"`
	ResumeDocxURI string     `json:"-"`
	CoverDocxURI  string     `json:"-"`
	HasResumeDocx bool       `json:"has_resume_docx"`
	HasCoverDocx  bool       `json:"has_cover_docx"`
	Status        string     `json:"status"`
	Error         string     `json:"error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type TailorCreate struct {
	JobID   *uuid.UUID
	Source  string
	Title   string
	Company string
	JDMD    string
}

func (s *Store) CreateTailorJob(ctx context.Context, in TailorCreate) (TailorJob, error) {
	if in.Source == "" {
		in.Source = "paste"
	}
	var t TailorJob
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tailor_jobs (job_id, source, title, company, jd_md, status)
		VALUES ($1, $2, $3, $4, $5, 'pending')
		RETURNING id, job_id, source, title, company, jd_md, resume_md, cover_letter_md,
		          notes_md, model, resume_docx_uri, cover_docx_uri, status, error, created_at, updated_at
	`, in.JobID, in.Source, in.Title, in.Company, in.JDMD).Scan(
		&t.ID, &t.JobID, &t.Source, &t.Title, &t.Company, &t.JDMD,
		&t.ResumeMD, &t.CoverLetterMD, &t.NotesMD, &t.Model, &t.ResumeDocxURI, &t.CoverDocxURI,
		&t.Status, &t.Error, &t.CreatedAt, &t.UpdatedAt,
	)
	t.HasResumeDocx = t.ResumeDocxURI != ""
	t.HasCoverDocx = t.CoverDocxURI != ""
	return t, err
}

func (s *Store) GetTailorJob(ctx context.Context, id uuid.UUID) (TailorJob, error) {
	t, err := scanTailor(s.pool.QueryRow(ctx, tailorSelect+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return TailorJob{}, ErrNotFound
	}
	return t, err
}

func (s *Store) ListTailorJobs(ctx context.Context) ([]TailorJob, error) {
	rows, err := s.pool.Query(ctx, tailorSelect+` ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TailorJob
	for rows.Next() {
		t, err := scanTailor(rows)
		if err != nil {
			return nil, err
		}
		t.JDMD = "" // list stays lean; GET /tailor/:id has the JD and drafts
		t.ResumeMD = ""
		t.CoverLetterMD = ""
		out = append(out, t)
	}
	if out == nil {
		out = []TailorJob{}
	}
	return out, rows.Err()
}

const tailorSelect = `
		SELECT id, job_id, source, title, company, jd_md, resume_md, cover_letter_md,
		       notes_md, model, resume_docx_uri, cover_docx_uri, status, error, created_at, updated_at
		FROM tailor_jobs
`

func scanTailor(row rowScanner) (TailorJob, error) {
	var t TailorJob
	err := row.Scan(
		&t.ID, &t.JobID, &t.Source, &t.Title, &t.Company, &t.JDMD,
		&t.ResumeMD, &t.CoverLetterMD, &t.NotesMD, &t.Model, &t.ResumeDocxURI, &t.CoverDocxURI,
		&t.Status, &t.Error, &t.CreatedAt, &t.UpdatedAt,
	)
	t.HasResumeDocx = t.ResumeDocxURI != ""
	t.HasCoverDocx = t.CoverDocxURI != ""
	return t, err
}
