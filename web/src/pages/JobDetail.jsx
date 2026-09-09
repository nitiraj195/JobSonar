import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "../api.js";

const STAGES = ["saved", "applied", "screen", "interview", "offer", "closed"];

function pct(n) {
  return n == null ? "—" : `${Math.round(n * 100)}%`;
}

function formatSalary(job) {
  if (job.salary_min == null && job.salary_max == null) return null;
  const cur = (job.currency || "").toUpperCase();
  const high = job.salary_max ?? job.salary_min;
  const inr = cur === "INR" || cur === "RS" || (!cur && /(india|pune|bengaluru|bangalore|hyderabad|mumbai|chennai|noida|gurgaon|gurugram|delhi|kolkata)/i.test(job.location || ""));
  if (inr && high < 1000) {
    const range = job.salary_min != null && job.salary_max != null && job.salary_min !== job.salary_max
      ? `${job.salary_min} – ${job.salary_max}`
      : `${high}`;
    return `₹${range} LPA`;
  }
  const fmt = (n) => new Intl.NumberFormat("en-IN", { maximumFractionDigits: 0 }).format(n);
  const range = job.salary_min != null && job.salary_max != null && job.salary_min !== job.salary_max
    ? `${fmt(job.salary_min)} – ${fmt(job.salary_max)}`
    : fmt(job.salary_min ?? job.salary_max);
  if (inr) return `₹${range}`;
  return cur ? `${cur} ${range}` : range;
}

const BAND_LABEL = {
  strong: "strong match",
  possible: "possible match",
  stretch: "stretch",
  excluded: "excluded by a hard gate",
};

export default function JobDetail({ profile }) {
  const { id } = useParams();
  const nav = useNavigate();
  const [job, setJob] = useState(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function load() {
    try {
      setJob(await api.job(id, profile));
    } catch (e) {
      setErr(e.message);
    }
  }
  useEffect(() => { if (profile) load(); }, [id, profile]);

  async function save() {
    setBusy(true);
    setErr("");
    try {
      const app = await api.saveJob(id, profile);
      await load();
      nav("/tracker");
      return app;
    } catch (e) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  }

  async function move(status) {
    if (!job?.application?.id) {
      await save();
      return;
    }
    setBusy(true);
    try {
      await api.moveApp(job.application.id, status);
      await load();
    } catch (e) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  }

  if (!job && !err) return <main><p>Loading…</p></main>;
  if (!job) return <main><p className="err">{err}</p></main>;

  const score = job.score;
  const band = score?.band || "unscored";

  return (
    <main className="detail">
      <Link to="/" className="back">← Jobs</Link>
      <h1>{job.title}</h1>
      <p className="sub">{job.company} · {job.location || "—"} · {job.source}{formatSalary(job) ? ` · ${formatSalary(job)}` : ""}</p>
      <div className="row">
        <div className="score big" data-band={band}>{score ? pct(score.composite) : "…"}</div>
        <div>
          <p>
            <strong>Match</strong>{" "}
            {score ? `${pct(score.composite)} · ${BAND_LABEL[band] || band}` : "not scored yet — the agent hasn't run against this profile (make agent, or make embed for one pass)"}
          </p>
          {band === "excluded" && (
            <p className="err">Excluded by a hard gate — a must-have skill, seniority, or location preference didn't match. The sub-scores below still show why.</p>
          )}
          {score && (
            <p className="chips">
              <span className="pipe">skill coverage {pct(score.skill_cov)}</span>
              <span className="pipe">semantic {pct(score.semantic)}</span>
              <span className="pipe">seniority {pct(score.seniority_fit)}</span>
              <span className="pipe">location {pct(score.location_fit)}</span>
              <span className="pipe">recency {pct(score.recency)}</span>
            </p>
          )}
          <p><strong>Matched</strong> {(score?.matched_skills || []).join(", ") || "—"}</p>
          <p><strong>Job asks, not on resume</strong> {(score?.missing_skills || []).join(", ") || "—"}</p>
          <p><strong>Salary</strong> {formatSalary(job) || "not posted"}</p>
        </div>
      </div>
      {job.review && (
        <section className="analysis review">
          <h2>Company and role reviews</h2>
          <p>
            {job.review.rating != null && <strong>{job.review.rating.toFixed(1)} / 5 · </strong>}
            {job.review.summary || "Open the links below. Glassdoor and Mouthshut do not offer a public read API."}
          </p>
          <p className="chips">
            {job.review.links?.glassdoor && <a className="pipe" href={job.review.links.glassdoor} target="_blank" rel="noreferrer">Glassdoor</a>}
            {job.review.links?.mouthshut && <a className="pipe" href={job.review.links.mouthshut} target="_blank" rel="noreferrer">Mouthshut</a>}
            {job.review.links?.web_search && <a className="pipe" href={job.review.links.web_search} target="_blank" rel="noreferrer">Web search</a>}
          </p>
          {(job.review.snippets || []).length > 0 && (
            <ul className="snippets">
              {job.review.snippets.map((s) => (
                <li key={s.url}>
                  <a href={s.url} target="_blank" rel="noreferrer">{s.title || s.source || s.url}</a>
                  {s.snippet && <p>{s.snippet}</p>}
                </li>
              ))}
            </ul>
          )}
          {job.review.provider && <p className="meta">Review lookup · {job.review.provider}{job.review.status === "error" ? " · search failed, links only" : ""}</p>}
        </section>
      )}
      {job.analysis && (
        <section className="analysis">
          <h2>Why you fit</h2>
          <p>{job.analysis.justification_md || "—"}</p>
          <h2>What to close</h2>
          <p>{job.analysis.tailoring_md || "—"}</p>
          {job.analysis.model && <p className="meta">Deep dive · {job.analysis.model}</p>}
        </section>
      )}
      {!job.analysis && band && band !== "strong" && band !== "unscored" && (
        <p className="meta">Deep dive runs on strong matches only.</p>
      )}
      <div className="actions">
        <a className="btn ghost" href={job.source_url} target="_blank" rel="noreferrer">Open posting</a>
        <Link className="btn ghost" to={`/tailor?job=${job.id}`}>Tailor resume + cover letter</Link>
        {!job.application && <button className="btn" disabled={busy} onClick={save}>Save to tracker</button>}
        {job.application && (
          <label>
            Pipeline
            <select value={job.application.status} disabled={busy} onChange={(e) => move(e.target.value)}>
              {STAGES.map((s) => <option key={s} value={s}>{s}</option>)}
            </select>
          </label>
        )}
      </div>
      {err && <p className="err">{err}</p>}
      <article className="desc">{job.description_md || "No description stored."}</article>
    </main>
  );
}
