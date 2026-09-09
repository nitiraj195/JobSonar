package handlers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/as76513/JobSonar/services/api/internal/reviews"
	"github.com/as76513/JobSonar/services/api/internal/store"
)

const maxResumeBytes = 5 << 20

type Jobs interface {
	ListJobs(ctx context.Context, profileID uuid.UUID, opts store.JobListOpts) ([]store.Job, error)
	GetJob(ctx context.Context, profileID, id uuid.UUID) (store.Job, error)
}

type ReviewCache interface {
	UpsertCompanyReview(ctx context.Context, r store.CompanyReview) (store.CompanyReview, error)
	ListSalaryJobCompanies(ctx context.Context) ([]store.CompanyRole, error)
}

type Companies interface {
	CreateCompany(ctx context.Context, name, ats, token string) (store.Company, error)
}

type Profiles interface {
	GetProfile(ctx context.Context, profileID uuid.UUID) (store.Profile, error)
	GetProfileByName(ctx context.Context, name string) (store.Profile, error)
	ListProfiles(ctx context.Context) ([]store.Profile, error)
	UpsertProfile(ctx context.Context, profileID uuid.UUID, skills []string) (store.Profile, error)
}

type Applications interface {
	ListApplications(ctx context.Context, profileID uuid.UUID) ([]store.Application, error)
	CreateApplication(ctx context.Context, profileID, jobID uuid.UUID) (store.Application, error)
	UpdateApplicationStatus(ctx context.Context, id uuid.UUID, status string) (store.Application, error)
}

type Resumes interface {
	CreateResume(ctx context.Context, profileID uuid.UUID, storageURI string) (store.Resume, error)
	LatestResume(ctx context.Context, profileID uuid.UUID) (store.Resume, error)
}

type Handler struct {
	jobs         Jobs
	companies    Companies
	profiles     Profiles
	applications Applications
	resumes      Resumes
	resumeDir    string
	searcher     reviews.Searcher
	reviews      ReviewCache
	tailor       TailorJobs
}

type TailorJobs interface {
	CreateTailorJob(ctx context.Context, in store.TailorCreate) (store.TailorJob, error)
	GetTailorJob(ctx context.Context, id uuid.UUID) (store.TailorJob, error)
	ListTailorJobs(ctx context.Context, profileID uuid.UUID) ([]store.TailorJob, error)
}

func New(jobs Jobs, companies Companies, profiles Profiles, applications Applications, resumes Resumes, resumeDir string) *Handler {
	return &Handler{
		jobs: jobs, companies: companies, profiles: profiles,
		applications: applications, resumes: resumes, resumeDir: resumeDir,
		searcher: reviews.LinkOnly{},
	}
}

func (h *Handler) WithTailor(t TailorJobs) *Handler {
	h.tailor = t
	return h
}

func (h *Handler) WithReviews(searcher reviews.Searcher, cache ReviewCache) *Handler {
	if searcher != nil {
		h.searcher = searcher
	}
	h.reviews = cache
	return h
}

func (h *Handler) Mount(app *fiber.App) {
	app.Get("/jobs", h.listJobs)
	app.Get("/jobs/:id", h.getJob)
	app.Post("/reviews/refresh", h.refreshReviews)
	app.Post("/companies", h.createCompany)
	app.Get("/profiles", h.listProfiles)
	app.Get("/profile", h.getProfile)
	app.Put("/profile", h.putProfile)
	app.Post("/profile/resume", h.uploadResume)
	app.Get("/tailor", h.listTailor)
	app.Post("/tailor", h.createTailor)
	app.Get("/tailor/:id/resume.docx", h.downloadTailorResume)
	app.Get("/tailor/:id/cover.docx", h.downloadTailorCover)
	app.Get("/tailor/:id", h.getTailor)
	app.Get("/applications", h.listApplications)
	app.Post("/applications", h.createApplication)
	app.Patch("/applications/:id", h.patchApplication)
}

// resolveProfile resolves the requesting profile from ?profile=<name>
// (Week 8: named multi-profile support). Falls back to DEFAULT_PROFILE_NAME,
// then to the first profile alphabetically, so existing callers that never
// pass ?profile= (older UI builds, curl, tests) keep working against
// whichever single profile exists locally.
func (h *Handler) resolveProfile(c *fiber.Ctx) (store.Profile, error) {
	name := strings.TrimSpace(c.Query("profile"))
	if name == "" {
		name = strings.TrimSpace(os.Getenv("DEFAULT_PROFILE_NAME"))
	}
	if name != "" {
		return h.profiles.GetProfileByName(c.Context(), name)
	}
	list, err := h.profiles.ListProfiles(c.Context())
	if err != nil {
		return store.Profile{}, err
	}
	if len(list) == 0 {
		return store.Profile{}, store.ErrNotFound
	}
	return list[0], nil
}

func profileErr(err error) error {
	if err == store.ErrNotFound {
		return fiber.NewError(fiber.StatusNotFound, "profile not found")
	}
	return fiber.NewError(fiber.StatusInternalServerError, err.Error())
}

func (h *Handler) listProfiles(c *fiber.Ctx) error {
	list, err := h.profiles.ListProfiles(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(list)
}

// listJobs and getJob no longer compute anything: ranking and every
// sub-score come from the `scores` table (Week 6), written by the agent's
// scoring pass and already ordered by store.ListJobs's SQL. A nil
// Job.Score means the agent hasn't scored it yet, not that it scored
// zero -- the web UI's existing "waiting on the agent" state covers that.

func parseJobListOpts(c *fiber.Ctx) (store.JobListOpts, error) {
	var opts store.JobListOpts
	switch strings.ToLower(strings.TrimSpace(c.Query("has_salary"))) {
	case "", "0", "false", "no":
	case "1", "true", "yes":
		opts.HasSalary = true
	default:
		return opts, fiber.NewError(fiber.StatusBadRequest, "has_salary must be 1 or 0")
	}
	sort := strings.ToLower(strings.TrimSpace(c.Query("sort")))
	switch sort {
	case "", "match":
		opts.Sort = "match"
	case "salary":
		opts.Sort = "salary"
	default:
		return opts, fiber.NewError(fiber.StatusBadRequest, "sort must be match or salary")
	}
	return opts, nil
}

func (h *Handler) listJobs(c *fiber.Ctx) error {
	p, err := h.resolveProfile(c)
	if err != nil {
		return profileErr(err)
	}
	opts, err := parseJobListOpts(c)
	if err != nil {
		return err
	}
	jobs, err := h.jobs.ListJobs(c.Context(), p.ID, opts)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	for i := range jobs {
		jobs[i].DescriptionMD = "" // list view omits the description; getJob includes it
		jobs[i].Analysis = nil     // keep has_analysis; omit prose from the list payload
		if jobs[i].Review != nil {
			jobs[i].Review.Snippets = nil
		} else {
			links, _ := reviews.LinkOnly{}.Search(c.Context(), jobs[i].Company, jobs[i].Title)
			jobs[i].Review = &links
			jobs[i].Review.Snippets = nil
		}
	}
	return c.JSON(jobs)
}

func (h *Handler) getJob(c *fiber.Ctx) error {
	p, err := h.resolveProfile(c)
	if err != nil {
		return profileErr(err)
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid job id")
	}
	job, err := h.jobs.GetJob(c.Context(), p.ID, id)
	if err != nil {
		if err == store.ErrNotFound {
			return fiber.NewError(fiber.StatusNotFound, "job not found")
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if rev := h.ensureReview(c.Context(), job); rev != nil {
		job.Review = rev
	}
	return c.JSON(job)
}

func (h *Handler) ensureReview(ctx context.Context, job store.Job) *store.CompanyReview {
	if !reviews.Stale(job.Review, time.Now()) {
		return job.Review
	}
	rev, err := h.searcher.Search(ctx, job.Company, job.Title)
	if err != nil {
		fallback, _ := reviews.LinkOnly{}.Search(ctx, job.Company, job.Title)
		fallback.Status = "error"
		fallback.Error = "review search failed"
		rev = fallback
	}
	if h.reviews != nil {
		if saved, err := h.reviews.UpsertCompanyReview(ctx, rev); err == nil {
			return &saved
		}
	}
	return &rev
}

func (h *Handler) refreshReviews(c *fiber.Ctx) error {
	if h.reviews == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "review cache not configured")
	}
	pairs, err := h.reviews.ListSalaryJobCompanies(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	const maxRefresh = 40
	n := 0
	for _, p := range pairs {
		if n >= maxRefresh {
			break
		}
		rev, err := h.searcher.Search(c.Context(), p.Company, p.Title)
		if err != nil {
			rev, _ = reviews.LinkOnly{}.Search(c.Context(), p.Company, p.Title)
			rev.Status = "error"
			rev.Error = "review search failed"
		}
		if _, err := h.reviews.UpsertCompanyReview(c.Context(), rev); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		n++
		if h.searcher.Name() == "brave" && n < len(pairs) && n < maxRefresh {
			time.Sleep(200 * time.Millisecond)
		}
	}
	return c.JSON(fiber.Map{
		"refreshed": n,
		"total":     len(pairs),
		"provider":  h.searcher.Name(),
	})
}

type createCompanyReq struct {
	Name       string `json:"name"`
	ATS        string `json:"ats"`
	BoardToken string `json:"board_token"`
}

func (h *Handler) createCompany(c *fiber.Ctx) error {
	var req createCompanyReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid json")
	}
	req.Name = strings.TrimSpace(req.Name)
	req.ATS = strings.ToLower(strings.TrimSpace(req.ATS))
	req.BoardToken = strings.TrimSpace(req.BoardToken)
	if req.Name == "" || req.BoardToken == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name and board_token are required")
	}
	if req.ATS != "greenhouse" {
		return fiber.NewError(fiber.StatusBadRequest, "ats must be greenhouse")
	}
	co, err := h.companies.CreateCompany(c.Context(), req.Name, req.ATS, req.BoardToken)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(co)
}

func (h *Handler) getProfile(c *fiber.Ctx) error {
	p, err := h.resolveProfile(c)
	if err != nil {
		return profileErr(err)
	}
	if h.resumes != nil {
		if r, err := h.resumes.LatestResume(c.Context(), p.ID); err == nil {
			p.LatestResume = &r
		}
	}
	return c.JSON(p)
}

func (h *Handler) uploadResume(c *fiber.Ctx) error {
	if h.resumes == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "resume upload not configured")
	}
	p, err := h.resolveProfile(c)
	if err != nil {
		return profileErr(err)
	}
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file is required")
	}
	if file.Size == 0 || file.Size > maxResumeBytes {
		return fiber.NewError(fiber.StatusBadRequest, "file must be between 1 byte and 5MB")
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".pdf" && ext != ".docx" {
		return fiber.NewError(fiber.StatusBadRequest, "file must be a PDF or DOCX")
	}
	dir := h.resumeDir
	if dir == "" {
		dir = "data/resumes"
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "could not store resume")
	}
	id := uuid.New()
	dest, err := filepath.Abs(filepath.Join(dir, id.String()+ext))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "could not store resume")
	}
	if err := c.SaveFile(file, dest); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "could not store resume")
	}
	row, err := h.resumes.CreateResume(c.Context(), p.ID, dest)
	if err != nil {
		_ = os.Remove(dest)
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(row)
}

type putProfileReq struct {
	Skills []string `json:"skills"`
}

func (h *Handler) putProfile(c *fiber.Ctx) error {
	existing, err := h.resolveProfile(c)
	if err != nil {
		return profileErr(err)
	}
	var req putProfileReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid json")
	}
	clean := make([]string, 0, len(req.Skills))
	seen := map[string]struct{}{}
	for _, s := range req.Skills {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		clean = append(clean, s)
	}
	p, err := h.profiles.UpsertProfile(c.Context(), existing.ID, clean)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(p)
}

func (h *Handler) listApplications(c *fiber.Ctx) error {
	p, err := h.resolveProfile(c)
	if err != nil {
		return profileErr(err)
	}
	apps, err := h.applications.ListApplications(c.Context(), p.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(apps)
}

type createAppReq struct {
	JobID string `json:"job_id"`
}

func (h *Handler) createApplication(c *fiber.Ctx) error {
	p, err := h.resolveProfile(c)
	if err != nil {
		return profileErr(err)
	}
	var req createAppReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid json")
	}
	jobID, err := uuid.Parse(strings.TrimSpace(req.JobID))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid job_id")
	}
	app, err := h.applications.CreateApplication(c.Context(), p.ID, jobID)
	if err != nil {
		if err == store.ErrNotFound {
			return fiber.NewError(fiber.StatusNotFound, "job not found")
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(app)
}

type patchAppReq struct {
	Status string `json:"status"`
}

func (h *Handler) patchApplication(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid application id")
	}
	var req patchAppReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid json")
	}
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if !store.ValidStatus(req.Status) {
		return fiber.NewError(fiber.StatusBadRequest, "status must be saved, applied, screen, interview, offer, or closed")
	}
	app, err := h.applications.UpdateApplicationStatus(c.Context(), id, req.Status)
	if err != nil {
		if err == store.ErrNotFound {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(app)
}

const maxJDBytes = 64 << 10

type createTailorReq struct {
	JobID   string `json:"job_id"`
	Title   string `json:"title"`
	Company string `json:"company"`
	JDMD    string `json:"jd_md"`
}

func (h *Handler) listTailor(c *fiber.Ctx) error {
	if h.tailor == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "tailor not configured")
	}
	p, err := h.resolveProfile(c)
	if err != nil {
		return profileErr(err)
	}
	rows, err := h.tailor.ListTailorJobs(c.Context(), p.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(rows)
}

func (h *Handler) getTailor(c *fiber.Ctx) error {
	if h.tailor == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "tailor not configured")
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid tailor id")
	}
	row, err := h.tailor.GetTailorJob(c.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			return fiber.NewError(fiber.StatusNotFound, "tailor job not found")
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(row)
}

func (h *Handler) downloadTailorResume(c *fiber.Ctx) error {
	return h.downloadTailorDocx(c, true)
}

func (h *Handler) downloadTailorCover(c *fiber.Ctx) error {
	return h.downloadTailorDocx(c, false)
}

func (h *Handler) downloadTailorDocx(c *fiber.Ctx, resume bool) error {
	if h.tailor == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "tailor not configured")
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid tailor id")
	}
	row, err := h.tailor.GetTailorJob(c.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			return fiber.NewError(fiber.StatusNotFound, "tailor job not found")
		}
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	path := row.ResumeDocxURI
	name := "resume-tailored.docx"
	if !resume {
		path = row.CoverDocxURI
		name = "cover-letter.docx"
	}
	if path == "" {
		return fiber.NewError(fiber.StatusNotFound, "DOCX not ready yet — wait for the agent, then refresh")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "DOCX not found")
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "DOCX not found")
	}
	c.Set(fiber.HeaderContentType, "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	c.Attachment(name)
	return c.Send(body)
}

func (h *Handler) createTailor(c *fiber.Ctx) error {
	if h.tailor == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "tailor not configured")
	}
	p, err := h.resolveProfile(c)
	if err != nil {
		return profileErr(err)
	}
	in := store.TailorCreate{ProfileID: p.ID, Source: "paste"}
	ct := strings.ToLower(c.Get("Content-Type"))
	if strings.HasPrefix(ct, "multipart/form-data") {
		in.Title = strings.TrimSpace(c.FormValue("title"))
		in.Company = strings.TrimSpace(c.FormValue("company"))
		in.JDMD = strings.TrimSpace(c.FormValue("jd_md"))
		if raw := strings.TrimSpace(c.FormValue("job_id")); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "invalid job_id")
			}
			in.JobID = &id
		}
		if file, err := c.FormFile("file"); err == nil && file != nil {
			ext := strings.ToLower(filepath.Ext(file.Filename))
			if ext != ".md" && ext != ".txt" {
				return fiber.NewError(fiber.StatusBadRequest, "file must be .md or .txt")
			}
			if file.Size == 0 || file.Size > maxJDBytes {
				return fiber.NewError(fiber.StatusBadRequest, "JD file must be between 1 byte and 64KB")
			}
			f, err := file.Open()
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "could not read JD file")
			}
			defer f.Close()
			body, err := io.ReadAll(io.LimitReader(f, maxJDBytes+1))
			if err != nil || len(body) == 0 {
				return fiber.NewError(fiber.StatusBadRequest, "could not read JD file")
			}
			in.JDMD = strings.TrimSpace(string(body))
			in.Source = "upload"
		}
	} else {
		var req createTailorReq
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid json")
		}
		in.Title = strings.TrimSpace(req.Title)
		in.Company = strings.TrimSpace(req.Company)
		in.JDMD = strings.TrimSpace(req.JDMD)
		if raw := strings.TrimSpace(req.JobID); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "invalid job_id")
			}
			in.JobID = &id
		}
	}
	if in.JobID != nil {
		job, err := h.jobs.GetJob(c.Context(), p.ID, *in.JobID)
		if err != nil {
			if err == store.ErrNotFound {
				return fiber.NewError(fiber.StatusNotFound, "job not found")
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if in.Title == "" {
			in.Title = job.Title
		}
		if in.Company == "" {
			in.Company = job.Company
		}
		if in.JDMD == "" {
			in.JDMD = strings.TrimSpace(job.DescriptionMD)
			in.Source = "job"
		}
	}
	if in.JDMD == "" {
		return fiber.NewError(fiber.StatusBadRequest, "jd_md, a .md/.txt file, or job_id is required")
	}
	if len(in.JDMD) > maxJDBytes {
		return fiber.NewError(fiber.StatusBadRequest, "JD must be at most 64KB")
	}
	row, err := h.tailor.CreateTailorJob(c.Context(), in)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(row)
}
