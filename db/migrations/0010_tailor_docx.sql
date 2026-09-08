-- +goose Up
-- Generated DOCX paths for tailor drafts. Files live under TAILOR_DIR
-- (gitignored); the API only streams them, never logs bytes.

ALTER TABLE tailor_jobs
    ADD COLUMN IF NOT EXISTS resume_docx_uri TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cover_docx_uri TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE tailor_jobs
    DROP COLUMN IF EXISTS resume_docx_uri,
    DROP COLUMN IF EXISTS cover_docx_uri;
