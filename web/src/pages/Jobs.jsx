import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api.js";

function pct(n) {
  return `${Math.round((n || 0) * 100)}%`;
}

function indiaLocation(loc) {
  return /(india|pune|bengaluru|bangalore|hyderabad|mumbai|chennai|noida|gurgaon|gurugram|delhi|kolkata)/i.test(loc || "");
}

function formatSalary(j) {
  if (j.salary_min == null && j.salary_max == null) return null;
  const cur = (j.currency || "").toUpperCase();
  const high = j.salary_max ?? j.salary_min;
  const inr = cur === "INR" || cur === "RS" || (!cur && indiaLocation(j.location));
  if (inr && high < 1000) {
    const range = j.salary_min != null && j.salary_max != null && j.salary_min !== j.salary_max
      ? `${j.salary_min}–${j.salary_max}`
      : `${high}`;
    return `₹${range} LPA`;
  }
  const fmt = (n) => {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
    if (n >= 1000) return `${Math.round(n / 1000)}k`;
    return `${Math.round(n)}`;
  };
  const range = j.salary_min != null && j.salary_max != null && j.salary_min !== j.salary_max
    ? `${fmt(j.salary_min)}–${fmt(j.salary_max)}`
    : fmt(j.salary_min ?? j.salary_max);
  if (inr) return `₹${range}`;
  return cur ? `${cur} ${range}` : range;
}

function snapshot(jobs) {
  const top = jobs[0];
  if (!top) return null;
  return {
    id: top.id,
    title: top.title,
    company: top.company,
    band: top.score?.band ?? null,
    score: top.score?.composite ?? 0,
  };
}

function pipeline(profile, jobs, phase) {
  const resume = profile?.latest_resume;
  const scored = (jobs || []).some((j) => j.score != null);
  const stored = Boolean(resume) || phase === "uploading" || phase === "waiting";
  const parsed = resume?.status === "done";
  const failed = resume?.status === "error";
  const embedded = Boolean(profile?.has_embedding);
  return [
    {
      id: "stored",
      label: "Resume stored",
      hint: "File saved locally. Nothing is sent to a cloud model.",
      state: phase === "uploading" ? "active" : stored ? "done" : "todo",
    },
    {
      id: "parsed",
      label: "Skills extracted",
      hint: failed
        ? (resume.error || "Parse failed")
        : parsed
          ? `${(profile.skills || []).length} skills from the resume`
          : "Agent reads the file (make embed or make agent)",
      state: failed ? "error" : parsed ? "done" : stored || phase === "waiting" ? "active" : "todo",
    },
    {
      id: "embedded",
      label: "Profile embedded",
      hint: embedded ? "Vector ready for the semantic sub-score" : "Same agent pass writes the profile vector",
      state: embedded ? "done" : parsed ? "active" : "todo",
    },
    {
      id: "scored",
      label: "Jobs scored",
      hint: scored ? "List ordered by composite score (skill coverage, semantic, seniority, location, recency)" : "Agent's scoring pass hasn't run against this profile yet",
      state: scored ? "done" : embedded ? "active" : "todo",
    },
  ];
}

export default function Jobs({ profile }) {
  const [jobs, setJobs] = useState([]);
  const [skills, setSkills] = useState("");
  const [profileInfo, setProfileInfo] = useState(null);
  const [err, setErr] = useState("");
  const [saving, setSaving] = useState(false);
  const [phase, setPhase] = useState("idle");
  const [notice, setNotice] = useState("");
  const [hasSalary, setHasSalary] = useState(false);
  const [sort, setSort] = useState("match");
  const [reviewBusy, setReviewBusy] = useState(false);
  const beforeRef = useRef(null);
  const watchStarted = useRef(0);

  async function refresh(next = {}) {
    const pay = next.hasSalary ?? hasSalary;
    const order = next.sort ?? sort;
    const [j, p] = await Promise.all([api.jobs({ hasSalary: pay, sort: order, profile }), api.profile(profile)]);
    setJobs(j || []);
    setProfileInfo(p);
    setSkills((p.skills || []).join(", "));
    return { jobs: j || [], profile: p };
  }

  useEffect(() => {
    if (!profile) return;
    refresh().then(({ profile: p }) => {
      if (p?.latest_resume?.status === "pending") {
        setPhase("waiting");
        watchStarted.current = Date.now();
      }
    }).catch((e) => setErr(e.message));
  }, [profile]);

  useEffect(() => {
    if (phase !== "waiting") return undefined;
    let stop = false;
    async function tick() {
      if (stop) return;
      try {
        const { jobs: nextJobs, profile: p } = await refresh();
        const st = p?.latest_resume?.status;
        if (st === "error") {
          setPhase("idle");
          setErr(p.latest_resume.error || "Resume parse failed");
          setNotice("");
          return;
        }
        const scored = st === "done" && p.has_embedding && nextJobs.some((j) => j.score != null);
        if (scored) {
          const before = beforeRef.current;
          const after = snapshot(nextJobs);
          if (before && after && before.id !== after.id) {
            setNotice(
              `Matching updated. Was “${before.title}” (${before.band || "unscored"}). Now “${after.title}” (${after.band || "unscored"} ${pct(after.score)}).`,
            );
          } else {
            setNotice("Matching updated from this resume. List is ranked by composite score.");
          }
          setPhase("idle");
          return;
        }
        if (st === "done" && p.has_embedding) {
          setNotice("Profile is embedded. Re-ranking jobs…");
        }
        if (Date.now() - watchStarted.current > 90000) {
          setNotice("Still waiting on the agent. In a terminal: make embed (one pass) or make agent (loop). This page will pick up the result.");
        }
      } catch (e) {
        // A single poll blip (API restart, migrate) must not freeze the
        // pipeline on "Internal Server Error" during embed/score.
        if (!stop) setNotice(e.message);
      }
    }
    tick();
    const id = setInterval(tick, 1000);
    return () => {
      stop = true;
      clearInterval(id);
    };
  }, [phase]);

  async function saveSkills(e) {
    e.preventDefault();
    setSaving(true);
    setErr("");
    try {
      const list = skills.split(",").map((s) => s.trim()).filter(Boolean);
      await api.saveProfile(list, profile);
      await refresh();
      setNotice("Skill list saved. Re-scoring waits until the next agent pass (make embed or make agent).");
    } catch (e) {
      setErr(e.message);
    } finally {
      setSaving(false);
    }
  }

  async function onUpload(e) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    setErr("");
    setNotice("");
    beforeRef.current = snapshot(jobs);
    setPhase("uploading");
    try {
      await api.uploadResume(file, profile);
      watchStarted.current = Date.now();
      setPhase("waiting");
    } catch (e) {
      setErr(e.message);
      setPhase("idle");
    }
  }

  function applySalaryFilter(on) {
    const order = on ? "salary" : "match";
    setHasSalary(on);
    setSort(order);
    refresh({ hasSalary: on, sort: order }).catch((e) => setErr(e.message));
  }

  function applySort(order) {
    setSort(order);
    refresh({ sort: order }).catch((e) => setErr(e.message));
  }

  async function onRefreshReviews() {
    setReviewBusy(true);
    setErr("");
    try {
      const r = await api.refreshReviews();
      await refresh();
      setNotice(`Reviews refreshed for ${r.refreshed} salary-listed roles (${r.provider}). Glassdoor/Mouthshut have no public API — this is search snippets plus outbound links.`);
    } catch (e) {
      setErr(e.message);
    } finally {
      setReviewBusy(false);
    }
  }

  const steps = pipeline(profileInfo, jobs, phase);
  const scored = jobs.some((j) => j.score != null);
  const busy = phase === "uploading" || phase === "waiting";
  const rankLabel = sort === "salary" ? "high pay (then reviews, then match)" : "composite score";

  return (
    <main>
      <section className="pipeline" aria-live="polite">
        <div className="pipeline-head">
          <h2>Match pipeline</h2>
          <p>
            {phase === "uploading" && "Uploading resume…"}
            {phase === "waiting" && "Waiting for the local agent to parse and embed. The job list refreshes when that finishes."}
            {phase === "idle" && (scored
              ? "Match % is a composite of skill coverage, semantic similarity, seniority, location, and recency — with a strong/possible/stretch band."
              : "Not scored yet. Upload a resume, or run the agent, to see match scores.")}
          </p>
        </div>
        <ol>
          {steps.map((s, i) => (
            <li key={s.id} data-state={s.state}>
              <span className="step-n">{s.state === "done" ? "✓" : i + 1}</span>
              <div>
                <strong>{s.label}</strong>
                <p>{s.hint}</p>
              </div>
            </li>
          ))}
        </ol>
        {notice && <p className="notice">{notice}</p>}
      </section>

      <form className="skills" onSubmit={saveSkills}>
        <label>
          Skill list (keyword override — does not re-embed)
          <input value={skills} onChange={(e) => setSkills(e.target.value)} placeholder="kubernetes, terraform, aws" />
        </label>
        <button type="submit" disabled={saving || busy}>{saving ? "Saving…" : "Re-rank keywords"}</button>
      </form>
      <div className="skills upload">
        <label>
          Resume (PDF or DOCX) — stored locally, parsed by the agent
          <input type="file" accept=".pdf,.docx,application/pdf" onChange={onUpload} disabled={busy} />
        </label>
      </div>
      {err && <p className="err">{err}</p>}
      <div className="filters">
        <label className="toggle">
          <input type="checkbox" checked={hasSalary} onChange={(e) => applySalaryFilter(e.target.checked)} />
          Salary listed
        </label>
        <label>
          Rank
          <select value={sort} onChange={(e) => applySort(e.target.value)}>
            <option value="match">Match score</option>
            <option value="salary">High pay first</option>
          </select>
        </label>
        <button type="button" className="btn ghost" disabled={reviewBusy || busy} onClick={onRefreshReviews}>
          {reviewBusy ? "Searching reviews…" : "Refresh reviews"}
        </button>
      </div>
      <p className="meta">
        {jobs.length} jobs · ranked by {rankLabel}
        {hasSalary ? " · salary required" : ""}
        {busy ? " · watching for agent…" : ""}
      </p>
      <ul className="cards">
        {jobs.map((j) => {
          const band = j.score?.band || "unscored";
          return (
            <li key={j.id}>
              <Link to={`/jobs/${j.id}`} className="card">
                <div className="score" data-band={band}>
                  {j.score ? pct(j.score.composite) : "…"}
                </div>
                <div>
                  <h2>{j.title}</h2>
                  <p>{j.company} · {j.location || "—"} · {j.source}{formatSalary(j) ? ` · ${formatSalary(j)}` : ""}</p>
                  <p className="chips">
                    {j.personalized_match && <span className="pipe personalized">🎯 personalized match</span>}
                    {j.score && <span className="pipe">{band}</span>}
                    {formatSalary(j) && <span className="pipe">{formatSalary(j)}</span>}
                    {j.review?.rating != null && <span className="pipe">reviews {j.review.rating.toFixed(1)}/5</span>}
                    {j.has_analysis && <span className="pipe">analyzed</span>}
                    {j.score?.semantic != null && <span className="pipe">sim {pct(j.score.semantic)}</span>}
                    {(j.score?.matched_skills || []).slice(0, 6).map((s) => <span key={s}>{s}</span>)}
                    {j.application && <span className="pipe">{j.application.status}</span>}
                  </p>
                </div>
              </Link>
            </li>
          );
        })}
      </ul>
    </main>
  );
}
