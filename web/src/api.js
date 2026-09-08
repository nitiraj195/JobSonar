async function req(path, opts = {}) {
  const headers = { ...(opts.headers || {}) };
  if (!(opts.body instanceof FormData) && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json";
  }
  let res;
  try {
    res = await fetch(path, { ...opts, headers });
  } catch {
    throw new Error("Cannot reach the API (make api on :8080, then reload).");
  }
  const text = await res.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      if (!res.ok) {
        throw new Error(res.statusText || "request failed");
      }
    }
  }
  if (!res.ok) {
    throw new Error(data?.error || res.statusText || "request failed");
  }
  return data;
}

export const api = {
  jobs: (opts = {}) => {
    const q = new URLSearchParams();
    if (opts.hasSalary) q.set("has_salary", "1");
    if (opts.sort) q.set("sort", opts.sort);
    const qs = q.toString();
    return req("/jobs" + (qs ? `?${qs}` : ""));
  },
  job: (id) => req(`/jobs/${id}`),
  refreshReviews: () => req("/reviews/refresh", { method: "POST" }),
  tailorJobs: () => req("/tailor"),
  tailorJob: (id) => req(`/tailor/${id}`),
  createTailor: (body) => req("/tailor", { method: "POST", body: JSON.stringify(body) }),
  createTailorUpload: (body) => req("/tailor", { method: "POST", body }),
  downloadTailorDocx: async (id, kind) => {
    const name = kind === "cover" ? "cover-letter.docx" : "resume-tailored.docx";
    let res;
    try {
      res = await fetch(`/tailor/${id}/${kind === "cover" ? "cover" : "resume"}.docx`);
    } catch {
      throw new Error("Cannot reach the API (make api on :8080, then reload).");
    }
    if (!res.ok) {
      const text = await res.text();
      let msg = res.statusText || "download failed";
      try {
        msg = JSON.parse(text)?.error || msg;
      } catch {
        if (text && !text.startsWith("<")) msg = text;
      }
      throw new Error(msg);
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  },
  profile: () => req("/profile"),
  saveProfile: (skills) => req("/profile", { method: "PUT", body: JSON.stringify({ skills }) }),
  uploadResume: (file) => {
    const body = new FormData();
    body.append("file", file);
    return req("/profile/resume", { method: "POST", body });
  },
  applications: () => req("/applications"),
  saveJob: (jobId) => req("/applications", { method: "POST", body: JSON.stringify({ job_id: jobId }) }),
  moveApp: (id, status) => req(`/applications/${id}`, { method: "PATCH", body: JSON.stringify({ status }) }),
};
