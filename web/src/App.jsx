import { useEffect, useState } from "react";
import { NavLink, Route, Routes } from "react-router-dom";
import Jobs from "./pages/Jobs.jsx";
import JobDetail from "./pages/JobDetail.jsx";
import Tailor from "./pages/Tailor.jsx";
import Tracker from "./pages/Tracker.jsx";
import { api } from "./api.js";

const PROFILE_STORAGE_KEY = "jobsonar.profile";

function storedProfile() {
  try {
    return localStorage.getItem(PROFILE_STORAGE_KEY) || "";
  } catch {
    return "";
  }
}

function storeProfile(name) {
  try {
    localStorage.setItem(PROFILE_STORAGE_KEY, name);
  } catch {
    // best-effort only -- a private window or blocked storage just means
    // the profile choice doesn't survive a reload.
  }
}

export default function App() {
  const [profiles, setProfiles] = useState([]);
  const [profile, setProfile] = useState(storedProfile());

  useEffect(() => {
    api.profiles().then((list) => {
      setProfiles(list || []);
      setProfile((current) => {
        if (current && (list || []).some((p) => p.name === current)) return current;
        return list?.[0]?.name || "";
      });
    }).catch(() => {});
  }, []);

  function selectProfile(name) {
    setProfile(name);
    storeProfile(name);
  }

  return (
    <div className="shell">
      <header className="top">
        <div>
          <p className="brand">JobSonar</p>
          <p className="tag">Ranked ingest · human-only apply</p>
        </div>
        <nav>
          <NavLink to="/" end>Jobs</NavLink>
          <NavLink to="/tailor">Tailor</NavLink>
          <NavLink to="/tracker">Tracker</NavLink>
        </nav>
        {profiles.length > 0 && (
          <label className="profile-switch">
            Profile
            <select value={profile} onChange={(e) => selectProfile(e.target.value)}>
              {profiles.map((p) => (
                <option key={p.name} value={p.name}>{p.name}</option>
              ))}
            </select>
          </label>
        )}
      </header>
      <Routes>
        <Route path="/" element={<Jobs profile={profile} />} />
        <Route path="/jobs/:id" element={<JobDetail profile={profile} />} />
        <Route path="/tailor" element={<Tailor profile={profile} />} />
        <Route path="/tracker" element={<Tracker profile={profile} />} />
      </Routes>
    </div>
  );
}
