import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api.js";

const COLS = ["saved", "applied", "screen", "interview", "offer", "closed"];

export default function Tracker({ profile }) {
  const [apps, setApps] = useState([]);
  const [err, setErr] = useState("");

  async function load() {
    try {
      setApps((await api.applications(profile)) || []);
    } catch (e) {
      setErr(e.message);
    }
  }
  useEffect(() => { if (profile) load(); }, [profile]);

  async function move(id, status) {
    try {
      await api.moveApp(id, status);
      await load();
    } catch (e) {
      setErr(e.message);
    }
  }

  return (
    <main>
      <h1>Tracker</h1>
      <p className="meta">Statuses are recorded only. JobSonar never submits an application.</p>
      {err && <p className="err">{err}</p>}
      <div className="board">
        {COLS.map((col) => (
          <section key={col} className="col">
            <h2>{col} <em>{apps.filter((a) => a.status === col).length}</em></h2>
            {apps.filter((a) => a.status === col).map((a) => (
              <article key={a.id} className="tile">
                <Link to={`/jobs/${a.job_id}`}>{a.title}</Link>
                <p>{a.company}</p>
                <select value={a.status} onChange={(e) => move(a.id, e.target.value)}>
                  {COLS.map((s) => <option key={s} value={s}>{s}</option>)}
                </select>
              </article>
            ))}
          </section>
        ))}
      </div>
    </main>
  );
}
