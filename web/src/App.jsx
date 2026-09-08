import { NavLink, Route, Routes } from "react-router-dom";
import Jobs from "./pages/Jobs.jsx";
import JobDetail from "./pages/JobDetail.jsx";
import Tailor from "./pages/Tailor.jsx";
import Tracker from "./pages/Tracker.jsx";

export default function App() {
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
      </header>
      <Routes>
        <Route path="/" element={<Jobs />} />
        <Route path="/jobs/:id" element={<JobDetail />} />
        <Route path="/tailor" element={<Tailor />} />
        <Route path="/tracker" element={<Tracker />} />
      </Routes>
    </div>
  );
}
