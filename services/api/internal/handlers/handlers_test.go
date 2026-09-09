package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/as76513/JobSonar/services/api/internal/store"
)

type fake struct {
	jobs      []store.Job
	companies []store.Company
	profile   store.Profile
	apps      []store.Application
	resumes   []store.Resume
	lastOpts  store.JobListOpts
	tailors   []store.TailorJob
}

func (f *fake) ListJobs(_ context.Context, _ uuid.UUID, opts store.JobListOpts) ([]store.Job, error) {
	f.lastOpts = opts
	return f.jobs, nil
}

func (f *fake) GetJob(_ context.Context, _ uuid.UUID, id uuid.UUID) (store.Job, error) {
	for _, j := range f.jobs {
		if j.ID == id {
			return j, nil
		}
	}
	return store.Job{}, store.ErrNotFound
}

func (f *fake) CreateCompany(_ context.Context, name, ats, token string) (store.Company, error) {
	c := store.Company{
		ID: uuid.New(), Name: name, ATS: ats, BoardToken: token, CreatedAt: time.Now().UTC(),
	}
	f.companies = append(f.companies, c)
	return c, nil
}

func (f *fake) GetProfile(context.Context, uuid.UUID) (store.Profile, error) {
	if f.profile.Skills == nil {
		f.profile.Skills = []string{}
	}
	return f.profile, nil
}

func (f *fake) GetProfileByName(context.Context, string) (store.Profile, error) {
	if f.profile.Skills == nil {
		f.profile.Skills = []string{}
	}
	return f.profile, nil
}

func (f *fake) ListProfiles(context.Context) ([]store.Profile, error) {
	if f.profile.Skills == nil {
		f.profile.Skills = []string{}
	}
	return []store.Profile{f.profile}, nil
}

func (f *fake) UpsertProfile(_ context.Context, _ uuid.UUID, skills []string) (store.Profile, error) {
	f.profile.Skills = skills
	f.profile.UpdatedAt = time.Now().UTC()
	if f.profile.ID == uuid.Nil {
		f.profile.ID = uuid.New()
	}
	return f.profile, nil
}

func (f *fake) ListApplications(context.Context, uuid.UUID) ([]store.Application, error) {
	return f.apps, nil
}

func (f *fake) CreateApplication(_ context.Context, _ uuid.UUID, jobID uuid.UUID) (store.Application, error) {
	var job store.Job
	found := false
	for _, j := range f.jobs {
		if j.ID == jobID {
			job, found = j, true
			break
		}
	}
	if !found {
		return store.Application{}, store.ErrNotFound
	}
	a := store.Application{
		ID: uuid.New(), JobID: jobID, Title: job.Title, Company: job.Company,
		Location: job.Location, SourceURL: job.SourceURL, Status: "saved", CreatedAt: time.Now().UTC(),
	}
	f.apps = append(f.apps, a)
	return a, nil
}

func (f *fake) CreateResume(_ context.Context, _ uuid.UUID, storageURI string) (store.Resume, error) {
	r := store.Resume{ID: uuid.New(), Status: "pending", CreatedAt: time.Now().UTC(), StorageURI: storageURI}
	f.resumes = append(f.resumes, r)
	return r, nil
}

func (f *fake) LatestResume(context.Context, uuid.UUID) (store.Resume, error) {
	if len(f.resumes) == 0 {
		return store.Resume{}, store.ErrNotFound
	}
	return f.resumes[len(f.resumes)-1], nil
}

func (f *fake) CreateTailorJob(_ context.Context, in store.TailorCreate) (store.TailorJob, error) {
	t := store.TailorJob{
		ID: uuid.New(), Source: in.Source, Title: in.Title, Company: in.Company,
		JDMD: in.JDMD, Status: "pending", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		JobID: in.JobID,
	}
	f.tailors = append(f.tailors, t)
	return t, nil
}

func (f *fake) GetTailorJob(_ context.Context, id uuid.UUID) (store.TailorJob, error) {
	for _, t := range f.tailors {
		if t.ID == id {
			return t, nil
		}
	}
	return store.TailorJob{}, store.ErrNotFound
}

func (f *fake) ListTailorJobs(context.Context, uuid.UUID) ([]store.TailorJob, error) {
	return f.tailors, nil
}

func (f *fake) UpdateApplicationStatus(_ context.Context, id uuid.UUID, status string) (store.Application, error) {
	for i, a := range f.apps {
		if a.ID == id {
			f.apps[i].Status = status
			return f.apps[i], nil
		}
	}
	return store.Application{}, store.ErrNotFound
}

func setup(t *testing.T, f *fake) *fiber.App {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
		}
		return c.Status(code).JSON(fiber.Map{"error": err.Error()})
	}})
	New(f, f, f, f, f, t.TempDir()).WithTailor(f).Mount(app)
	return app
}

func TestListAndGetJobs(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	f := &fake{
		profile: store.Profile{Skills: []string{"kubernetes", "sales"}},
		jobs: []store.Job{{
			ID: id, Title: "DevOps Engineer", Company: "Example", Location: "Amsterdam",
			Source: "adzuna", SourceURL: "https://example.test/1", Status: "open",
			DescriptionMD: "Run kubernetes clusters.",
			Sources: []store.JobSource{
				{Source: "adzuna", SourceURL: "https://example.test/1"},
				{Source: "greenhouse", SourceURL: "https://boards.greenhouse.io/x/jobs/1"},
			},
			Score: &store.Score{
				Composite: 0.8, SkillCov: 1, SeniorityFit: 1, LocationFit: 1, Recency: 1,
				Band: "strong", MatchedSkills: []string{"kubernetes"}, MissingSkills: []string{},
			},
		}},
	}
	app := setup(t, f)

	resp, err := app.Test(httptest.NewRequest("GET", "/jobs", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var listed []map[string]any
	if err := json.Unmarshal(body, &listed); err != nil || len(listed) != 1 {
		t.Fatalf("list=%s err=%v", body, err)
	}
	score, _ := listed[0]["score"].(map[string]any)
	if score == nil || score["band"] != "strong" {
		t.Fatalf("want score.band=strong (passed through from the store, not recomputed): %s", body)
	}
	if listed[0]["description_md"] != nil && listed[0]["description_md"] != "" {
		t.Fatal("list should omit description")
	}

	resp, err = app.Test(httptest.NewRequest("GET", "/jobs/"+id.String(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("get status=%d", resp.StatusCode)
	}

	resp, err = app.Test(httptest.NewRequest("GET", "/jobs/"+uuid.NewString(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 404 {
		t.Fatalf("missing status=%d", resp.StatusCode)
	}
}

func TestGetJobReturnsAnalysis(t *testing.T) {
	id := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	f := &fake{jobs: []store.Job{{
		ID: id, Title: "SRE", Company: "Acme", Location: "Pune",
		Score:       &store.Score{Composite: 0.85, Band: "strong"},
		HasAnalysis: true,
		Analysis: &store.Analysis{
			JustificationMD: "You fit kubernetes.",
			TailoringMD:     "Add a Helm bullet.",
			Model:           "fake",
		},
	}}}
	app := setup(t, f)

	resp, err := app.Test(httptest.NewRequest("GET", "/jobs/"+id.String(), nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	an, _ := got["analysis"].(map[string]any)
	if an == nil || an["justification_md"] != "You fit kubernetes." || an["tailoring_md"] != "Add a Helm bullet." {
		t.Fatalf("detail should include analysis: %s", body)
	}

	resp, err = app.Test(httptest.NewRequest("GET", "/jobs", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	var listed []map[string]any
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatal(err)
	}
	if listed[0]["analysis"] != nil {
		t.Fatalf("list should omit analysis prose: %s", body)
	}
}

func TestCreateCompany(t *testing.T) {
	f := &fake{}
	app := setup(t, f)

	req := httptest.NewRequest("POST", "/companies", bytes.NewBufferString(
		`{"name":"Stripe","ats":"greenhouse","board_token":"stripe"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if len(f.companies) != 1 || f.companies[0].BoardToken != "stripe" {
		t.Fatalf("%+v", f.companies)
	}

	bad := httptest.NewRequest("POST", "/companies", bytes.NewBufferString(
		`{"name":"X","ats":"lever","board_token":"x"}`))
	bad.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(bad)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("lever status=%d", resp.StatusCode)
	}
}

func TestApplicationsPipeline(t *testing.T) {
	jobID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	f := &fake{jobs: []store.Job{{
		ID: jobID, Title: "DevOps", Company: "Acme", Location: "Pune", SourceURL: "https://x.test",
	}}}
	app := setup(t, f)

	req := httptest.NewRequest("POST", "/applications", bytes.NewBufferString(`{"job_id":"`+jobID.String()+`"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("create status=%d", resp.StatusCode)
	}
	var created store.Application
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &created); err != nil || created.Status != "saved" {
		t.Fatalf("%s", body)
	}

	patch := httptest.NewRequest("PATCH", "/applications/"+created.ID.String(), bytes.NewBufferString(`{"status":"applied"}`))
	patch.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(patch)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("patch status=%d", resp.StatusCode)
	}

	bad := httptest.NewRequest("PATCH", "/applications/"+created.ID.String(), bytes.NewBufferString(`{"status":"ghosted"}`))
	bad.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(bad)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("ghosted status=%d", resp.StatusCode)
	}
}

// Week 6: ranking moved entirely into store.ListJobs's SQL (ORDER BY
// composite, tested live against Postgres in
// internal/store/scores_test.go). The handler must not re-sort or
// otherwise second-guess the order the store returns.
func TestListJobsSalaryQuery(t *testing.T) {
	f := &fake{jobs: []store.Job{{
		ID:    uuid.MustParse("66666666-6666-6666-6666-666666666666"),
		Title: "SRE", Company: "Acme",
	}}}
	app := setup(t, f)

	resp, err := app.Test(httptest.NewRequest("GET", "/jobs?has_salary=1&sort=salary", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if !f.lastOpts.HasSalary || f.lastOpts.Sort != "salary" {
		t.Fatalf("opts=%+v", f.lastOpts)
	}

	bad, err := app.Test(httptest.NewRequest("GET", "/jobs?sort=random", nil))
	if err != nil {
		t.Fatal(err)
	}
	if bad.StatusCode != 400 {
		t.Fatalf("bad sort status=%d", bad.StatusCode)
	}
}

func TestGetJobAttachesReviewLinks(t *testing.T) {
	id := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	f := &fake{jobs: []store.Job{{
		ID: id, Title: "DevOps Engineer", Company: "Dkatalis Labs",
	}}}
	app := setup(t, f)

	resp, err := app.Test(httptest.NewRequest("GET", "/jobs/"+id.String(), nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	rev, _ := got["review"].(map[string]any)
	links, _ := rev["links"].(map[string]any)
	if links == nil || links["glassdoor"] == nil || links["mouthshut"] == nil || links["web_search"] == nil {
		t.Fatalf("want outbound review links: %s", body)
	}
}

func TestListPreservesStoreOrder(t *testing.T) {
	second := store.Job{
		ID:    uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Title: "Sales Lead", Company: "Acme", Location: "Pune",
		Score: &store.Score{Composite: 0.2, Band: "stretch"},
	}
	first := store.Job{
		ID:    uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		Title: "Platform Engineer", Company: "Acme", Location: "Pune",
		Score: &store.Score{Composite: 0.9, Band: "strong"},
	}
	// Deliberately fed in store order (first, second), not sorted by the
	// handler -- if the handler re-sorted, this order would still come
	// out right, so what this actually proves is that it doesn't invert
	// or otherwise touch it.
	f := &fake{jobs: []store.Job{first, second}}
	app := setup(t, f)

	resp, err := app.Test(httptest.NewRequest("GET", "/jobs", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	var listed []map[string]any
	if err := json.Unmarshal(body, &listed); err != nil || len(listed) != 2 {
		t.Fatalf("%s", body)
	}
	if listed[0]["id"] != first.ID.String() || listed[1]["id"] != second.ID.String() {
		t.Fatalf("want store order preserved (first, second), got %s", body)
	}
}

func TestUploadResume(t *testing.T) {
	f := &fake{}
	app := setup(t, f)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "resume.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("%PDF-1.4 fake"))
	_ = w.Close()
	req := httptest.NewRequest("POST", "/profile/resume", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 202 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d %s", resp.StatusCode, body)
	}
	if len(f.resumes) != 1 || f.resumes[0].Status != "pending" {
		t.Fatalf("%+v", f.resumes)
	}

	bad := httptest.NewRequest("POST", "/profile/resume", bytes.NewBufferString("nope"))
	bad.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(bad)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("bad status=%d", resp.StatusCode)
	}

	var txt bytes.Buffer
	tw := multipart.NewWriter(&txt)
	part, _ = tw.CreateFormFile("file", "notes.txt")
	_, _ = part.Write([]byte("hello"))
	_ = tw.Close()
	rej := httptest.NewRequest("POST", "/profile/resume", &txt)
	rej.Header.Set("Content-Type", tw.FormDataContentType())
	resp, err = app.Test(rej)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("txt status=%d", resp.StatusCode)
	}
}

func TestCreateTailorFromPasteAndJob(t *testing.T) {
	jobID := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	f := &fake{jobs: []store.Job{{
		ID: jobID, Title: "SRE", Company: "Acme", DescriptionMD: "Need kubernetes and terraform.",
	}}}
	app := setup(t, f)

	req := httptest.NewRequest("POST", "/tailor", bytes.NewBufferString(
		`{"title":"DevOps","company":"Example","jd_md":"# Role\nNeed AWS and Kubernetes."}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 202 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("paste status=%d %s", resp.StatusCode, body)
	}
	if len(f.tailors) != 1 || f.tailors[0].Status != "pending" || !strings.Contains(f.tailors[0].JDMD, "Kubernetes") {
		t.Fatalf("%+v", f.tailors)
	}

	fromJob := httptest.NewRequest("POST", "/tailor", bytes.NewBufferString(`{"job_id":"`+jobID.String()+`"}`))
	fromJob.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(fromJob)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 202 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("job status=%d %s", resp.StatusCode, body)
	}
	if f.tailors[1].Title != "SRE" || f.tailors[1].Source != "job" {
		t.Fatalf("%+v", f.tailors[1])
	}

	empty := httptest.NewRequest("POST", "/tailor", bytes.NewBufferString(`{}`))
	empty.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(empty)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("empty status=%d", resp.StatusCode)
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("file", "jd.md")
	_, _ = part.Write([]byte("# Staff SRE\nOwn the on-call rotation."))
	_ = w.WriteField("company", "Acme")
	_ = w.Close()
	up := httptest.NewRequest("POST", "/tailor", &buf)
	up.Header.Set("Content-Type", w.FormDataContentType())
	resp, err = app.Test(up)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 202 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status=%d %s", resp.StatusCode, body)
	}
	if f.tailors[2].Source != "upload" || !strings.Contains(f.tailors[2].JDMD, "on-call") {
		t.Fatalf("%+v", f.tailors[2])
	}

	docx, err := app.Test(httptest.NewRequest("GET", "/tailor/"+f.tailors[0].ID.String()+"/resume.docx", nil))
	if err != nil {
		t.Fatal(err)
	}
	if docx.StatusCode != 404 {
		t.Fatalf("docx before agent status=%d", docx.StatusCode)
	}

	list, err := app.Test(httptest.NewRequest("GET", "/tailor", nil))
	if err != nil {
		t.Fatal(err)
	}
	if list.StatusCode != 200 {
		t.Fatalf("list status=%d", list.StatusCode)
	}
}
