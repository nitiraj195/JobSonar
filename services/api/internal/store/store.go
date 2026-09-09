package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type JobSource struct {
	Source    string `json:"source"`
	SourceURL string `json:"source_url"`
}

// Score mirrors one `scores` row (Week 6). Written by the agent's scoring
// pass (services/agent/jobsonar_agent/run.py:score_jobs); the API only
// reads it -- see docs/TRD.md §4.1's note on where normalisation/scoring
// reasoning lives. Nil on a Job means the agent hasn't scored it yet, not
// that it scored zero.
type Score struct {
	Composite     float64  `json:"composite"`
	SkillCov      float64  `json:"skill_cov"`
	Semantic      *float64 `json:"semantic,omitempty"`
	SeniorityFit  float64  `json:"seniority_fit"`
	LocationFit   float64  `json:"location_fit"`
	Recency       float64  `json:"recency"`
	Band          string   `json:"band"`
	MatchedSkills []string `json:"matched_skills"`
	MissingSkills []string `json:"missing_skills"`
}

type Job struct {
	ID            uuid.UUID      `json:"id"`
	Title         string         `json:"title"`
	Company       string         `json:"company"`
	Location      string         `json:"location"`
	Source        string         `json:"source"`
	SourceURL     string         `json:"source_url"`
	DescriptionMD string         `json:"description_md,omitempty"`
	SalaryMin     *float64       `json:"salary_min,omitempty"`
	SalaryMax     *float64       `json:"salary_max,omitempty"`
	Currency      string         `json:"currency,omitempty"`
	PostedAt      *time.Time     `json:"posted_at,omitempty"`
	Status        string         `json:"status"`
	LastSeenAt    time.Time      `json:"last_seen_at"`
	Sources       []JobSource    `json:"sources"`
	Application   *AppBrief      `json:"application,omitempty"`
	Score         *Score         `json:"score,omitempty"`
	HasAnalysis   bool           `json:"has_analysis"`
	Analysis      *Analysis      `json:"analysis,omitempty"`
	Review        *CompanyReview `json:"review,omitempty"`
	// PersonalizedMatch is true when the requesting profile's own
	// personalized web search (Week 8) surfaced this job -- see
	// job_discoveries. Independent of Score/Analysis.
	PersonalizedMatch bool `json:"personalized_match"`
}

// JobListOpts filters and reorders GET /jobs. Ranking stays in SQL.
type JobListOpts struct {
	HasSalary bool   // only rows with salary_min or salary_max
	Sort      string // "" / "match" (composite) or "salary" (high pay first)
}

type Analysis struct {
	JustificationMD string    `json:"justification_md"`
	TailoringMD     string    `json:"tailoring_md"`
	Model           string    `json:"model"`
	CreatedAt       time.Time `json:"created_at"`
}

type AppBrief struct {
	ID     uuid.UUID `json:"id"`
	Status string    `json:"status"`
}

type Resume struct {
	ID         uuid.UUID `json:"id"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	StorageURI string    `json:"-"`
}

type Profile struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Skills       []string  `json:"skills"`
	HasEmbedding bool      `json:"has_embedding"`
	Embedding    []float64 `json:"-"`
	LatestResume *Resume   `json:"latest_resume,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Application struct {
	ID            uuid.UUID  `json:"id"`
	JobID         uuid.UUID  `json:"job_id"`
	Title         string     `json:"title"`
	Company       string     `json:"company"`
	Location      string     `json:"location"`
	SourceURL     string     `json:"source_url"`
	ResumeVariant string     `json:"resume_variant"`
	Status        string     `json:"status"`
	AppliedAt     *time.Time `json:"applied_at,omitempty"`
	Notes         string     `json:"notes"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Company struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	ATS        string    `json:"ats"`
	BoardToken string    `json:"board_token"`
	CreatedAt  time.Time `json:"created_at"`
}

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) ListJobs(ctx context.Context, profileID uuid.UUID, opts JobListOpts) ([]Job, error) {
	q := jobSelect + jobFrom + `
		WHERE sc.band IS DISTINCT FROM 'excluded'`
	if opts.HasSalary {
		q += `
		AND (j.salary_min IS NOT NULL OR j.salary_max IS NOT NULL)`
	}
	q += jobOrder(opts.Sort)
	rows, err := s.pool.Query(ctx, q, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobs(rows)
}

func (s *Store) GetJob(ctx context.Context, profileID, id uuid.UUID) (Job, error) {
	// Unlike ListJobs, an excluded (hard-gated) score is still returned
	// here -- a job someone links to directly should explain why it was
	// excluded, not disappear (Week 6 Day 4: never silently dropped).
	row := s.pool.QueryRow(ctx, jobSelect+jobFrom+`
		WHERE j.id = $2
	`, profileID, id)
	j, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return j, err
}

const jobSelect = `
		SELECT j.id, j.title, j.company, j.location, j.source, j.source_url,
		       j.description_md, j.salary_min, j.salary_max, COALESCE(j.currency, ''),
		       j.posted_at, j.status, j.last_seen_at,
		       srcs.sources,
		       a.id, a.status,
		       sc.composite, sc.skill_cov, sc.semantic, sc.seniority_fit,
		       sc.location_fit, sc.recency, sc.band, sc.matched_skills, sc.missing_skills,
		       COALESCE(an.has_analysis, false),
		       an.justification_md, an.tailoring_md, an.model, an.created_at,
		       rev.rating, COALESCE(rev.review_count, 0), COALESCE(rev.summary, ''),
		       rev.snippets, rev.links, COALESCE(rev.provider, ''), COALESCE(rev.status, ''),
		       rev.fetched_at,
		       COALESCE(disc.personalized_match, false)
`

// jobFrom takes the requesting profile's id as $1 -- every caller (ListJobs,
// GetJob) must pass it as the first bound parameter.
const jobFrom = `
		FROM jobs j
		LEFT JOIN applications a ON a.job_id = j.id AND a.profile_id = $1
		LEFT JOIN LATERAL (
			SELECT COALESCE(json_agg(json_build_object('source', src.source, 'source_url', src.source_url)), '[]') AS sources
			FROM job_sources src WHERE src.job_id = j.id
		) srcs ON true
		LEFT JOIN LATERAL (
			SELECT * FROM scores sc
			WHERE sc.job_id = j.id AND sc.profile_id = $1
			LIMIT 1
		) sc ON true
		LEFT JOIN LATERAL (
			SELECT true AS has_analysis, justification_md, tailoring_md, model, created_at
			FROM analyses an
			WHERE an.job_id = j.id AND an.profile_id = $1
			LIMIT 1
		) an ON true
		LEFT JOIN company_reviews rev
		  ON rev.company_key = lower(trim(j.company))
		 AND rev.role_key = lower(trim(j.title))
		LEFT JOIN LATERAL (
			SELECT true AS personalized_match
			FROM job_discoveries jd
			WHERE jd.job_id = j.id AND jd.profile_id = $1
			LIMIT 1
		) disc ON true
`

// salaryUSDExpr converts posted ranges to an approximate USD figure so
// INR / EUR / GBP ranks compare. Missing currency + an India location
// is treated as INR (Adzuna IN often omits currency). Static FX is for
// ORDER BY only — the API still returns the source numbers.
const salaryUSDExpr = `COALESCE(j.salary_max, j.salary_min)
		* CASE
			WHEN (UPPER(TRIM(COALESCE(j.currency, ''))) IN ('INR', 'RS')
			      OR (COALESCE(TRIM(j.currency), '') = ''
			          AND j.location ~* '(india|pune|bengaluru|bangalore|hyderabad|mumbai|chennai|noida|gurgaon|gurugram|delhi|kolkata)'))
			     AND COALESCE(j.salary_max, j.salary_min) < 1000
			THEN 100000
			ELSE 1
		  END
		* CASE
			WHEN UPPER(TRIM(COALESCE(j.currency, ''))) IN ('INR', 'RS') THEN 0.012
			WHEN UPPER(TRIM(COALESCE(j.currency, ''))) IN ('EUR') THEN 1.08
			WHEN UPPER(TRIM(COALESCE(j.currency, ''))) IN ('GBP') THEN 1.27
			WHEN UPPER(TRIM(COALESCE(j.currency, ''))) = 'CAD' THEN 0.73
			WHEN UPPER(TRIM(COALESCE(j.currency, ''))) = 'AUD' THEN 0.66
			WHEN UPPER(TRIM(COALESCE(j.currency, ''))) = 'SGD' THEN 0.74
			WHEN COALESCE(TRIM(j.currency), '') = ''
			     AND j.location ~* '(india|pune|bengaluru|bangalore|hyderabad|mumbai|chennai|noida|gurgaon|gurugram|delhi|kolkata)'
			     THEN 0.012
			ELSE 1.0
		  END`

func jobOrder(sort string) string {
	if strings.EqualFold(sort, "salary") {
		return `
		ORDER BY ` + salaryUSDExpr + ` DESC NULLS LAST,
		         rev.rating DESC NULLS LAST,
		         sc.composite DESC NULLS LAST,
		         j.last_seen_at DESC`
	}
	return `
		ORDER BY sc.composite DESC NULLS LAST, j.last_seen_at DESC`
}

func (s *Store) CreateCompany(ctx context.Context, name, ats, token string) (Company, error) {
	var c Company
	err := s.pool.QueryRow(ctx, `
		INSERT INTO companies (name, ats, board_token)
		VALUES ($1, $2, $3)
		ON CONFLICT (ats, board_token) DO UPDATE SET name = EXCLUDED.name
		RETURNING id, name, ats, board_token, created_at
	`, name, ats, token).Scan(&c.ID, &c.Name, &c.ATS, &c.BoardToken, &c.CreatedAt)
	return c, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (Job, error) {
	var j Job
	var posted *time.Time
	var sources []byte
	var appID *uuid.UUID
	var appStatus *string
	var composite, skillCov, seniorityFit, locationFit, recency *float64
	var semantic *float64
	var band *string
	var matchedRaw, missingRaw []byte
	var hasAnalysis bool
	var just, tail, model *string
	var analysisAt *time.Time
	var revRating *float64
	var revCount int
	var revSummary, revProvider, revStatus string
	var revSnips, revLinks []byte
	var revFetched *time.Time
	if err := row.Scan(&j.ID, &j.Title, &j.Company, &j.Location, &j.Source, &j.SourceURL,
		&j.DescriptionMD, &j.SalaryMin, &j.SalaryMax, &j.Currency, &posted, &j.Status, &j.LastSeenAt, &sources, &appID, &appStatus,
		&composite, &skillCov, &semantic, &seniorityFit, &locationFit, &recency, &band, &matchedRaw, &missingRaw,
		&hasAnalysis, &just, &tail, &model, &analysisAt,
		&revRating, &revCount, &revSummary, &revSnips, &revLinks, &revProvider, &revStatus, &revFetched,
		&j.PersonalizedMatch); err != nil {
		return Job{}, err
	}
	j.HasAnalysis = hasAnalysis
	j.PostedAt = posted
	if err := jsonUnmarshalSources(sources, &j.Sources); err != nil {
		return Job{}, err
	}
	if j.Sources == nil {
		j.Sources = []JobSource{}
	}
	if appID != nil && appStatus != nil {
		j.Application = &AppBrief{ID: *appID, Status: *appStatus}
	}
	if composite != nil && band != nil {
		sc := &Score{
			Composite: *composite, Semantic: semantic, Band: *band,
		}
		if skillCov != nil {
			sc.SkillCov = *skillCov
		}
		if seniorityFit != nil {
			sc.SeniorityFit = *seniorityFit
		}
		if locationFit != nil {
			sc.LocationFit = *locationFit
		}
		if recency != nil {
			sc.Recency = *recency
		}
		if err := unmarshalJSON(matchedRaw, &sc.MatchedSkills); err != nil {
			return Job{}, err
		}
		if err := unmarshalJSON(missingRaw, &sc.MissingSkills); err != nil {
			return Job{}, err
		}
		if sc.MatchedSkills == nil {
			sc.MatchedSkills = []string{}
		}
		if sc.MissingSkills == nil {
			sc.MissingSkills = []string{}
		}
		j.Score = sc
	}
	if just != nil || tail != nil {
		a := &Analysis{}
		if just != nil {
			a.JustificationMD = *just
		}
		if tail != nil {
			a.TailoringMD = *tail
		}
		if model != nil {
			a.Model = *model
		}
		if analysisAt != nil {
			a.CreatedAt = *analysisAt
		}
		j.Analysis = a
		j.HasAnalysis = true
	}
	if revStatus != "" || len(revLinks) > 0 {
		rev := &CompanyReview{
			Company:     j.Company,
			RoleTitle:   j.Title,
			Rating:      revRating,
			ReviewCount: revCount,
			Summary:     revSummary,
			Provider:    revProvider,
			Status:      revStatus,
		}
		if err := unmarshalJSON(revSnips, &rev.Snippets); err != nil && len(revSnips) > 0 {
			return Job{}, err
		}
		if rev.Snippets == nil {
			rev.Snippets = []ReviewSnippet{}
		}
		if len(revLinks) > 0 {
			if err := unmarshalJSON(revLinks, &rev.Links); err != nil {
				return Job{}, err
			}
		}
		if revFetched != nil {
			rev.FetchedAt = *revFetched
		}
		j.Review = rev
	}
	return j, nil
}

func scanJobs(rows pgx.Rows) ([]Job, error) {
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	if out == nil {
		out = []Job{}
	}
	return out, rows.Err()
}

func jsonUnmarshalSources(b []byte, dest *[]JobSource) error {
	if len(b) == 0 {
		*dest = []JobSource{}
		return nil
	}
	return unmarshalJSON(b, dest)
}

func FormatVector(v []float64) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(x, 'f', -1, 64))
	}
	b.WriteByte(']')
	return b.String()
}

func ParseVector(s string) ([]float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		x, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}
