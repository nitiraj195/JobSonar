-- Week 8: named multi-profile seed data for nitiraj + amol (0011_named_profiles.sql
-- creates the two rows; this fills in skills/seniority/location curated from
-- profiles/*_profile_and_agent_prompt.md). Applied by `make seed`. Safe to re-run.

UPDATE profiles
SET seniority = 'lead', location = 'Pune', remote_pref = 'remote', updated_at = now()
WHERE name = 'nitiraj' AND seniority IS NULL;

UPDATE profiles
SET
    skills = '["kubernetes","terraform","aws","docker","ci/cd","python","go","golang",
               "security","vault","iam","sre","platform","kafka","snowflake","sql",
               "jenkins","gitlab","tls","cloudformation","lambda","linux"]'::jsonb,
    seniority = 'principal',
    location = 'Pune',
    remote_pref = 'remote',
    updated_at = now()
WHERE name = 'amol' AND skills = '[]'::jsonb;
