import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "../api.js";

async function copy(text) {
  await navigator.clipboard.writeText(text || "");
}

function renderInline(text, keyBase) {
  return text.split(/(\*\*.+?\*\*)/g).map((part, idx) =>
    part.startsWith("**") && part.endsWith("**") && part.length > 4
      ? <strong key={`${keyBase}-${idx}`}>{part.slice(2, -2)}</strong>
      : <span key={`${keyBase}-${idx}`}>{part}</span>
  );
}

// Mirrors the agent's markdown -> DOCX convention (tailor/docx.py) so the
// preview looks like the file the user is about to download.
function renderTailorMarkdown(md) {
  const lines = (md || "").split("\n");
  const out = [];
  let bullets = [];
  const flushBullets = () => {
    if (!bullets.length) return;
    out.push(<ul key={`ul-${out.length}`}>{bullets.map((b, idx) => <li key={idx}>{renderInline(b, `b${out.length}-${idx}`)}</li>)}</ul>);
    bullets = [];
  };
  let i = 0;
  while (i < lines.length) {
    const stripped = lines[i].trim();
    if (!stripped) { flushBullets(); i += 1; continue; }
    if (stripped.startsWith("# ")) {
      flushBullets();
      out.push(<h3 key={`n-${i}`} className="tailor-name">{renderInline(stripped.slice(2).trim(), `n${i}`)}</h3>);
      i += 1;
      let j = i;
      while (j < lines.length && !lines[j].trim()) j += 1;
      if (j < lines.length && !/^(#|- |\* |---)/.test(lines[j].trim())) {
        out.push(<p key={`c-${j}`} className="tailor-contact">{lines[j].trim()}</p>);
        i = j + 1;
      }
      continue;
    }
    if (stripped === "---") { flushBullets(); out.push(<hr key={`hr-${i}`} />); i += 1; continue; }
    if (stripped.startsWith("## ")) {
      flushBullets();
      out.push(<h4 key={`s-${i}`} className="tailor-section">{stripped.slice(3).trim().toUpperCase()}</h4>);
      i += 1;
      continue;
    }
    if (stripped.startsWith("- ") || stripped.startsWith("* ")) {
      bullets.push(stripped.slice(2).trim());
      i += 1;
      continue;
    }
    if (stripped.length > 2 && stripped.startsWith("_") && stripped.endsWith("_")) {
      flushBullets();
      out.push(<p key={`i-${i}`} className="tailor-footnote">{stripped.slice(1, -1).trim()}</p>);
      i += 1;
      continue;
    }
    flushBullets();
    out.push(<p key={`p-${i}`} className={stripped.startsWith("**") ? "tailor-entry" : undefined}>{renderInline(stripped, `p${i}`)}</p>);
    i += 1;
  }
  flushBullets();
  return out;
}

export default function Tailor() {
  const [params] = useSearchParams();
  const prefillJob = params.get("job") || "";
  const [jobs, setJobs] = useState([]);
  const [profile, setProfile] = useState(null);
  const [drafts, setDrafts] = useState([]);
  const [active, setActive] = useState(null);
  const [title, setTitle] = useState("");
  const [company, setCompany] = useState("");
  const [jobId, setJobId] = useState(prefillJob);
  const [jd, setJd] = useState("");
  const [err, setErr] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  async function refreshList() {
    const rows = (await api.tailorJobs()) || [];
    setDrafts(rows);
    return rows;
  }

  async function openDraft(id) {
    const row = await api.tailorJob(id);
    setActive(row);
    return row;
  }

  useEffect(() => {
    Promise.all([api.jobs(), api.profile(), refreshList()])
      .then(async ([j, p, rows]) => {
        setJobs(j || []);
        setProfile(p);
        const latest = (rows || []).find((r) => r.status === "done") || (rows || [])[0];
        if (latest) await openDraft(latest.id);
      })
      .catch((e) => setErr(e.message));
  }, []);

  useEffect(() => {
    if (!prefillJob) return;
    api.job(prefillJob).then((j) => {
      setTitle(j.title || "");
      setCompany(j.company || "");
      setJd(j.description_md || "");
      setJobId(j.id);
    }).catch((e) => setErr(e.message));
  }, [prefillJob]);

  useEffect(() => {
    if (!active || (active.status !== "pending")) return undefined;
    let stop = false;
    const id = setInterval(async () => {
      if (stop) return;
      try {
        const next = await openDraft(active.id);
        if (next.status !== "pending") {
          await refreshList();
          setBusy(false);
          setNotice(next.status === "done"
            ? "Draft ready. Review it — JobSonar never submits an application."
            : (next.error || "Tailor failed"));
        }
      } catch (e) {
        if (!stop) setErr(e.message);
      }
    }, 1000);
    return () => {
      stop = true;
      clearInterval(id);
    };
  }, [active?.id, active?.status]);

  async function submit(e) {
    e.preventDefault();
    setErr("");
    setNotice("");
    setBusy(true);
    try {
      const created = await api.createTailor({
        title,
        company,
        job_id: jobId,
        jd_md: jd,
      });
      setActive(created);
      await refreshList();
      setNotice("Queued. The local agent writes the drafts (make agent or make embed).");
    } catch (e) {
      setErr(e.message);
      setBusy(false);
    }
  }

  async function onFile(e) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    setErr("");
    setNotice("");
    setBusy(true);
    try {
      const body = new FormData();
      body.append("file", file);
      if (title) body.append("title", title);
      if (company) body.append("company", company);
      if (jobId) body.append("job_id", jobId);
      const created = await api.createTailorUpload(body);
      setActive(created);
      await refreshList();
      setNotice("JD file queued. Waiting on the local agent.");
    } catch (e) {
      setErr(e.message);
      setBusy(false);
    }
  }

  const resumeReady = profile?.latest_resume?.status === "done";

  return (
    <main className="tailor">
      <h1>Tailor</h1>
      <p className="meta">
        Paste a job description or upload a <code>.md</code> / <code>.txt</code> file.
        When the agent finishes, download the tailored <strong>DOCX</strong> resume and cover letter.
        Nothing is submitted.
      </p>
      {!resumeReady && (
        <p className="err">Upload and parse a resume on the Jobs page first — the agent needs that file.</p>
      )}
      <form className="skills tailor-form" onSubmit={submit}>
        <label>
          Title
          <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Staff SRE" />
        </label>
        <label>
          Company
          <input value={company} onChange={(e) => setCompany(e.target.value)} placeholder="Acme" />
        </label>
        <label>
          Or pick a stored job
          <select value={jobId} onChange={(e) => {
            const id = e.target.value;
            setJobId(id);
            const j = jobs.find((x) => x.id === id);
            if (j) {
              setTitle(j.title || "");
              setCompany(j.company || "");
              api.job(id).then((full) => setJd(full.description_md || "")).catch((err) => setErr(err.message));
            }
          }}>
            <option value="">—</option>
            {[...jobs].sort((a, b) => (a.title || "").localeCompare(b.title || "")).slice(0, 60).map((j) => (
              <option key={j.id} value={j.id}>{j.title} · {j.company}</option>
            ))}
          </select>
        </label>
        <label className="wide">
          Job description
          <textarea value={jd} onChange={(e) => setJd(e.target.value)} rows={10} placeholder="Paste the JD in markdown or plain text" />
        </label>
        <div className="tailor-actions">
          <button type="submit" disabled={busy || !jd.trim()}>{busy ? "Working…" : "Draft resume + cover letter"}</button>
          <label className="btn ghost">
            Upload .md / .txt
            <input type="file" accept=".md,.txt,text/markdown,text/plain" onChange={onFile} disabled={busy} hidden />
          </label>
        </div>
      </form>
      {err && <p className="err">{err}</p>}
      {notice && <p className="notice">{notice}</p>}

      {active && (
        <section className="analysis">
          <h2>{active.title || "Draft"} {active.company ? `· ${active.company}` : ""} · {active.status}</h2>
          {active.status === "pending" && <p className="meta">Waiting on the agent. In a terminal: <code>make embed</code> or <code>make agent</code>.</p>}
          {active.status === "error" && <p className="err">{active.error || "failed"}</p>}
          {active.status === "done" && (
            <>
              {active.model?.includes("fallback") && (
                <p className="notice fallback-notice">
                  Deterministic fallback draft — the local LLM was unavailable or too short, so JobSonar
                  reordered your resume text instead of rewriting it. Start Ollama (<code>make agent</code>)
                  and re-submit the JD for a stronger rewrite.
                </p>
              )}
              <div className="tailor-actions" style={{ marginBottom: "0.85rem" }}>
                {active.has_resume_docx
                  ? (
                    <button
                      type="button"
                      className="btn"
                      onClick={() => api.downloadTailorDocx(active.id, "resume").catch((e) => setErr(e.message))}
                    >
                      Download resume (DOCX)
                    </button>
                  )
                  : <span className="meta">Resume DOCX not ready yet — wait a few seconds and click the draft again.</span>}
                {active.has_cover_docx && (
                  <button
                    type="button"
                    className="btn ghost"
                    onClick={() => api.downloadTailorDocx(active.id, "cover").catch((e) => setErr(e.message))}
                  >
                    Download cover letter (DOCX)
                  </button>
                )}
              </div>
              <div className="tailor-cols">
                <article>
                  <div className="tailor-head">
                    <h3>Tailored resume</h3>
                    <button type="button" className="btn ghost" onClick={() => copy(active.resume_md)}>Copy text</button>
                  </div>
                  <div className="desc tailor-preview">
                    {active.resume_md ? renderTailorMarkdown(active.resume_md) : "—"}
                  </div>
                </article>
                <article>
                  <div className="tailor-head">
                    <h3>Cover letter</h3>
                    <button type="button" className="btn ghost" onClick={() => copy(active.cover_letter_md)}>Copy text</button>
                  </div>
                  <div className="desc tailor-preview">
                    {active.cover_letter_md ? renderTailorMarkdown(active.cover_letter_md) : "—"}
                  </div>
                </article>
              </div>
              {active.notes_md && (
                <>
                  <h3>Notes</h3>
                  <p>{active.notes_md}</p>
                </>
              )}
              {active.model && <p className="meta">Local draft · {active.model}</p>}
            </>
          )}
        </section>
      )}

      <h2 className="subhead">Recent drafts</h2>
      <ul className="cards">
        {drafts.map((d) => (
          <li key={d.id}>
            <button type="button" className="card" onClick={() => openDraft(d.id).catch((e) => setErr(e.message))}>
              <div>
                <h2>{d.title || "Untitled JD"}</h2>
                <p>{d.company || "—"} · {d.status} · {d.source}{d.has_resume_docx ? " · DOCX ready" : ""}</p>
              </div>
            </button>
          </li>
        ))}
      </ul>
      <p className="meta"><Link to="/">← Jobs</Link></p>
    </main>
  );
}
