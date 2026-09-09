package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

// newTestStore connects to a real Postgres (the docker-compose one, via
// make up && make migrate) and skips if POSTGRES_DSN isn't set, so `go
// test ./...` stays offline by default -- same convention as
// services/worker/internal/store's newTestStore.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run store tests against a live Postgres (see .env.example)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

// seedJobWithScore inserts a throwaway job and a scores row for it against
// a fresh profile, cleaned up by t.Cleanup.
func seedJobWithScore(t *testing.T, s *Store, profileID uuid.UUID, composite float64, band string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	jobID := uuid.New()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO jobs (id, dedup_hash, source, source_url, title, company, location)
		VALUES ($1, $2, 'test', $3, 'Test Job', 'Acme Score Test', 'Pune')
	`, jobID, "score-test-"+jobID.String(), "https://example.com/"+jobID.String())
	if err != nil {
		t.Fatalf("seed job: %v", err)
	}
	t.Cleanup(func() { _, _ = s.pool.Exec(context.Background(), "DELETE FROM jobs WHERE id = $1", jobID) })

	_, err = s.pool.Exec(ctx, `
		INSERT INTO scores (job_id, profile_id, composite, skill_cov, seniority_fit, location_fit, recency, band)
		VALUES ($1, $2, $3, $3, 1, 1, 1, $4)
	`, jobID, profileID, composite, band)
	if err != nil {
		t.Fatalf("seed score: %v", err)
	}
	return jobID
}

func seedProfile(t *testing.T, s *Store) uuid.UUID {
	t.Helper()
	profileID := uuid.New()
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO profiles (id, name, skills, updated_at) VALUES ($1, $2, '[]'::jsonb, now())
	`, profileID, "test-"+profileID.String())
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	t.Cleanup(func() { _, _ = s.pool.Exec(context.Background(), "DELETE FROM profiles WHERE id = $1", profileID) })
	return profileID
}

func TestListJobs_OrdersByCompositeAndExcludesGatedBand(t *testing.T) {
	s := newTestStore(t)
	profileID := seedProfile(t, s) // most-recently-updated profile -> "current" for scoring

	strongID := seedJobWithScore(t, s, profileID, 0.90, "strong")
	stretchID := seedJobWithScore(t, s, profileID, 0.20, "stretch")
	excludedID := seedJobWithScore(t, s, profileID, 0.99, "excluded") // high composite, still gated

	jobs, err := s.ListJobs(context.Background(), profileID, JobListOpts{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}

	var seen []uuid.UUID
	for _, j := range jobs {
		if j.ID == strongID || j.ID == stretchID || j.ID == excludedID {
			seen = append(seen, j.ID)
		}
	}
	if len(seen) != 2 || seen[0] != strongID || seen[1] != stretchID {
		t.Fatalf("want [strong, stretch] (excluded omitted) in that order, got %v", seen)
	}

	// GetJob, unlike ListJobs, must still surface the excluded job -- Week
	// 6 Day 4: gated jobs are never silently dropped, only hidden from
	// the default ranked list.
	excluded, err := s.GetJob(context.Background(), profileID, excludedID)
	if err != nil {
		t.Fatalf("GetJob(excluded): %v", err)
	}
	if excluded.Score == nil || excluded.Score.Band != "excluded" {
		t.Fatalf("want excluded job's Score.Band = excluded, got %+v", excluded.Score)
	}
}

func TestGetJob_IncludesAnalysis(t *testing.T) {
	s := newTestStore(t)
	profileID := seedProfile(t, s)
	jobID := seedJobWithScore(t, s, profileID, 0.88, "strong")
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO analyses (job_id, profile_id, justification_md, tailoring_md, model)
		VALUES ($1, $2, 'why fit', 'what to close', 'fake')
	`, jobID, profileID)
	if err != nil {
		t.Fatalf("seed analysis: %v", err)
	}

	job, err := s.GetJob(ctx, profileID, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if !job.HasAnalysis || job.Analysis == nil || job.Analysis.JustificationMD != "why fit" {
		t.Fatalf("want analysis on detail, got %+v", job.Analysis)
	}

	jobs, err := s.ListJobs(ctx, profileID, JobListOpts{})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	found := false
	for _, j := range jobs {
		if j.ID == jobID {
			found = true
			if !j.HasAnalysis {
				t.Fatal("list should set has_analysis")
			}
		}
	}
	if !found {
		t.Fatal("strong analyzed job missing from list")
	}
}

func TestListJobs_UnscoredJobHasNilScore(t *testing.T) {
	s := newTestStore(t)
	profileID := seedProfile(t, s)
	ctx := context.Background()
	jobID := uuid.New()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO jobs (id, dedup_hash, source, source_url, title, company, location)
		VALUES ($1, $2, 'test', $3, 'Unscored Job', 'Acme Score Test', 'Pune')
	`, jobID, "score-test-unscored-"+jobID.String(), "https://example.com/"+jobID.String())
	if err != nil {
		t.Fatalf("seed job: %v", err)
	}
	t.Cleanup(func() { _, _ = s.pool.Exec(context.Background(), "DELETE FROM jobs WHERE id = $1", jobID) })

	job, err := s.GetJob(ctx, profileID, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if job.Score != nil {
		t.Fatalf("want nil Score for an unscored job, got %+v", job.Score)
	}
}

// Week 8: personalized_match reflects only the *requesting* profile's own
// job_discoveries row -- a job discovered for one profile must not show
// as a match for another.
func TestGetJob_PersonalizedMatchIsPerProfile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	profileA := seedProfile(t, s)
	profileB := seedProfile(t, s)
	jobID := uuid.New()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO jobs (id, dedup_hash, source, source_url, title, company, location)
		VALUES ($1, $2, 'personalized-search', $3, 'Discovered Job', 'Acme Discover Test', 'Pune')
	`, jobID, "discover-test-"+jobID.String(), "https://example.com/"+jobID.String())
	if err != nil {
		t.Fatalf("seed job: %v", err)
	}
	t.Cleanup(func() { _, _ = s.pool.Exec(context.Background(), "DELETE FROM jobs WHERE id = $1", jobID) })

	_, err = s.pool.Exec(ctx, `
		INSERT INTO job_discoveries (job_id, profile_id) VALUES ($1, $2)
	`, jobID, profileA)
	if err != nil {
		t.Fatalf("seed job_discoveries: %v", err)
	}

	got, err := s.GetJob(ctx, profileA, jobID)
	if err != nil {
		t.Fatalf("GetJob(profileA): %v", err)
	}
	if !got.PersonalizedMatch {
		t.Fatal("want personalized_match=true for the profile that discovered it")
	}

	got, err = s.GetJob(ctx, profileB, jobID)
	if err != nil {
		t.Fatalf("GetJob(profileB): %v", err)
	}
	if got.PersonalizedMatch {
		t.Fatal("want personalized_match=false for a different profile")
	}

	jobs, err := s.ListJobs(ctx, profileA, JobListOpts{})
	if err != nil {
		t.Fatalf("ListJobs(profileA): %v", err)
	}
	found := false
	for _, j := range jobs {
		if j.ID == jobID {
			found = true
			if !j.PersonalizedMatch {
				t.Fatal("want personalized_match=true in the list view too")
			}
		}
	}
	if !found {
		t.Fatal("discovered job missing from ListJobs")
	}
}

func seedJobWithSalary(t *testing.T, s *Store, profileID uuid.UUID, composite float64, band string, min, max float64, currency, location string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	jobID := uuid.New()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO jobs (id, dedup_hash, source, source_url, title, company, location, salary_min, salary_max, currency)
		VALUES ($1, $2, 'test', $3, 'Paid Job', 'Pay Test Co', $4, $5, $6, $7)
	`, jobID, "pay-test-"+jobID.String(), "https://example.com/"+jobID.String(), location, min, max, currency)
	if err != nil {
		t.Fatalf("seed paid job: %v", err)
	}
	t.Cleanup(func() { _, _ = s.pool.Exec(context.Background(), "DELETE FROM jobs WHERE id = $1", jobID) })
	_, err = s.pool.Exec(ctx, `
		INSERT INTO scores (job_id, profile_id, composite, skill_cov, seniority_fit, location_fit, recency, band)
		VALUES ($1, $2, $3, $3, 1, 1, 1, $4)
	`, jobID, profileID, composite, band)
	if err != nil {
		t.Fatalf("seed score: %v", err)
	}
	return jobID
}

func TestListJobs_SalaryFilterAndHighPayOrder(t *testing.T) {
	s := newTestStore(t)
	profileID := seedProfile(t, s)

	// Higher match, lower pay.
	lowPay := seedJobWithSalary(t, s, profileID, 0.95, "strong", 50000, 60000, "USD", "Austin")
	// Lower match, higher pay.
	highPay := seedJobWithSalary(t, s, profileID, 0.40, "possible", 150000, 180000, "USD", "Austin")
	// Large INR number that must not outrank USD 150k after FX (~₹2M ≈ $24k).
	inr := seedJobWithSalary(t, s, profileID, 0.50, "possible", 2000000, 2000000, "INR", "Pune")
	// Adzuna IN often posts lakhs (25 LPA) without currency — treat as ₹2.5M, still below $150k.
	lpa := seedJobWithSalary(t, s, profileID, 0.55, "possible", 20, 25, "", "Pune")
	unpaid := seedJobWithScore(t, s, profileID, 0.99, "strong")

	filtered, err := s.ListJobs(context.Background(), profileID, JobListOpts{HasSalary: true, Sort: "match"})
	if err != nil {
		t.Fatalf("ListJobs has_salary: %v", err)
	}
	var got []uuid.UUID
	for _, j := range filtered {
		if j.ID == lowPay || j.ID == highPay || j.ID == inr || j.ID == lpa || j.ID == unpaid {
			got = append(got, j.ID)
		}
	}
	if len(got) != 4 {
		t.Fatalf("has_salary should drop unpaid, got %v", got)
	}
	if got[0] != lowPay {
		t.Fatalf("sort=match should still lead with highest composite (lowPay), got %v", got)
	}

	ranked, err := s.ListJobs(context.Background(), profileID, JobListOpts{HasSalary: true, Sort: "salary"})
	if err != nil {
		t.Fatalf("ListJobs sort=salary: %v", err)
	}
	got = nil
	for _, j := range ranked {
		if j.ID == lowPay || j.ID == highPay || j.ID == inr || j.ID == lpa {
			got = append(got, j.ID)
		}
	}
	if len(got) != 4 || got[0] != highPay || got[1] != lowPay || got[2] != lpa || got[3] != inr {
		t.Fatalf("want high USD, mid USD, 25 LPA, then ₹2M, got %v", got)
	}
	for _, j := range ranked {
		if j.ID == highPay && (j.SalaryMin == nil || *j.SalaryMin != 150000) {
			t.Fatalf("salary fields missing on API row: %+v", j)
		}
	}
}
