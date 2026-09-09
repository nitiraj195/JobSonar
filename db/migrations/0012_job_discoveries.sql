-- +goose Up
-- Week 8: personalized web search (agent-driven, per profile, opt-in).
-- job_discoveries records which profile's search surfaced a job, purely
-- additive -- job_sources' contract (job_id, source, source_url) is
-- untouched. profiles.last_personalized_search_at drives the schedule
-- (checked inside the agent's existing polling loop; no k8s CronJob
-- infra exists in this repo yet).

CREATE TABLE job_discoveries (
    job_id        UUID NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    profile_id    UUID NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    source        TEXT NOT NULL DEFAULT 'personalized-search',
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, profile_id)
);

CREATE INDEX job_discoveries_profile ON job_discoveries (profile_id, discovered_at DESC);

ALTER TABLE profiles ADD COLUMN IF NOT EXISTS last_personalized_search_at TIMESTAMPTZ;

-- Bug fix found while building this: skills_extracted defaulted to '[]'
-- (not NULL) on insert, but run.score_jobs only re-extracts when it sees
-- NULL ("None = never extracted" -- see its comment). So every brand-new
-- job, from any source, silently got skill_cov=0 forever. Dropping the
-- default/NOT NULL makes a fresh row genuinely NULL, matching that
-- intent. Does not touch already-populated rows.
ALTER TABLE jobs ALTER COLUMN skills_extracted DROP DEFAULT;
ALTER TABLE jobs ALTER COLUMN skills_extracted DROP NOT NULL;

-- +goose Down
ALTER TABLE jobs ALTER COLUMN skills_extracted SET NOT NULL;
ALTER TABLE jobs ALTER COLUMN skills_extracted SET DEFAULT '[]'::jsonb;

ALTER TABLE profiles DROP COLUMN IF EXISTS last_personalized_search_at;

DROP TABLE IF EXISTS job_discoveries;
