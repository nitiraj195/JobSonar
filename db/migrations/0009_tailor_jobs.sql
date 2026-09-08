-- +goose Up
-- On-demand JD → tailored resume + cover letter (FR-23 / FR-25).
-- Go stores the request; the Python agent writes the drafts. Assist-to-apply
-- only — never submitted. Resume source stays on disk (resumes.storage_uri);
-- this table holds the JD the seeker pasted/uploaded and the generated markdown.

CREATE TABLE tailor_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id      UUID REFERENCES profiles (id) ON DELETE SET NULL,
    resume_id       UUID REFERENCES resumes (id) ON DELETE SET NULL,
    job_id          UUID REFERENCES jobs (id) ON DELETE SET NULL,
    source          TEXT NOT NULL DEFAULT 'paste',
    title           TEXT NOT NULL DEFAULT '',
    company         TEXT NOT NULL DEFAULT '',
    jd_md           TEXT NOT NULL,
    resume_md       TEXT NOT NULL DEFAULT '',
    cover_letter_md TEXT NOT NULL DEFAULT '',
    notes_md        TEXT NOT NULL DEFAULT '',
    model           TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'pending',
    error           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX tailor_jobs_status_created ON tailor_jobs (status, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS tailor_jobs;
