-- +goose Up
-- Week 8: named multi-profile support (Amol + Nitiraj share one JobSonar
-- instance). profiles gets an identity; resumes and applications become
-- profile-scoped instead of "whichever row is most recent" (CLAUDE.md's
-- old single-user assumption). The one pre-existing local profile row
-- (if any) becomes 'nitiraj' -- this repo's owner -- and a fresh empty
-- 'amol' profile is added; existing resumes/applications backfill to
-- nitiraj's profile id.

ALTER TABLE profiles ADD COLUMN IF NOT EXISTS name TEXT;

UPDATE profiles SET name = 'nitiraj' WHERE name IS NULL;

INSERT INTO profiles (name, skills)
SELECT 'nitiraj', '[]'::jsonb
WHERE NOT EXISTS (SELECT 1 FROM profiles);

INSERT INTO profiles (name, skills)
SELECT 'amol', '[]'::jsonb
WHERE NOT EXISTS (SELECT 1 FROM profiles WHERE name = 'amol');

ALTER TABLE profiles ALTER COLUMN name SET NOT NULL;
ALTER TABLE profiles ADD CONSTRAINT profiles_name_key UNIQUE (name);

ALTER TABLE resumes ADD COLUMN IF NOT EXISTS profile_id UUID REFERENCES profiles (id) ON DELETE CASCADE;
UPDATE resumes SET profile_id = (SELECT id FROM profiles WHERE name = 'nitiraj') WHERE profile_id IS NULL;
ALTER TABLE resumes ALTER COLUMN profile_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS resumes_profile_created ON resumes (profile_id, created_at DESC);

ALTER TABLE applications ADD COLUMN IF NOT EXISTS profile_id UUID REFERENCES profiles (id) ON DELETE CASCADE;
UPDATE applications SET profile_id = (SELECT id FROM profiles WHERE name = 'nitiraj') WHERE profile_id IS NULL;
ALTER TABLE applications ALTER COLUMN profile_id SET NOT NULL;
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_job_id_key;
ALTER TABLE applications ADD CONSTRAINT applications_job_id_profile_id_key UNIQUE (job_id, profile_id);

-- +goose Down
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_job_id_profile_id_key;
ALTER TABLE applications ADD CONSTRAINT applications_job_id_key UNIQUE (job_id);
ALTER TABLE applications DROP COLUMN IF EXISTS profile_id;

DROP INDEX IF EXISTS resumes_profile_created;
ALTER TABLE resumes DROP COLUMN IF EXISTS profile_id;

ALTER TABLE profiles DROP CONSTRAINT IF EXISTS profiles_name_key;
ALTER TABLE profiles DROP COLUMN IF EXISTS name;
