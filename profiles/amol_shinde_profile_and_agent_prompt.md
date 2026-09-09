# Professional Profile, Work Summary & Claude Research Agent Prompt

## Candidate Overview

* **Name**: Amol Shinde
* **Title**: Product Security Engineer V – Platform Security / DevSecOps Architect
* **Experience**: 15+ Years in Software Engineering, Cloud Architecture, DevSecOps, and Application Security
* **Organization**: Avalara (Security Engineering / Product Security)
* **Education**: Master of Engineering (Information Technology), University of Mumbai; Bachelor of Engineering (Computer Engineering), University of Pune
* **Certifications**: 
  * AWS Certified Security – Specialty
  * AWS Certified AI Practitioner
  * AWS Certified Solutions Architect – Associate
  * AWS Certified Developer – Associate
  * HashiCorp Certified Terraform Associate
  * Certified Ethical Hacker (CEH) – in progress
  * Claude Certified Architect & AWS Certified Generative AI Developer tracks – active learning

---

## 2-Year Work Summary & Domain Categorization (2024–2026)

### 1. Security Data Engineering & AppSec Pipeline Automation (SDS / SDP)
* **Unified Security Data Sync (SDS) Architecture**: Architected and deployed microservices to ingest, normalize, and process vulnerability findings across diverse scanning tools—including Static Application Security Testing (SAST / Semgrep), Dynamic Application Security Testing (DAST / Burp Suite), Software Composition Analysis and Software Age (SCA / Endor Labs), Secret Detection (Gitleaks), and 3rd-Party Container Vulnerabilities (Wiz / 3PV).
* **Data Ingestion & Polling (DIP) Services**: Built resilient ingestion engines using Golang and Python deployed on Kubernetes clusters, integrated with Confluent Kafka message streams and Snowflake enterprise data warehousing.
* **DBT (Data Build Tool) Modeling & Data Normalization**: Created multi-layered DBT pipelines (Bronze, Silver, Gold layers) executing automated deduplication (`ava_dedup_group`), active container image matching, CWE-to-CVSS v4 rescoring, severity normalization, and bi-directional sync to push triaged false positives back into source security tools.
* **Automated Ticketing & Remediation Service**: Engineered a template-driven Jira ticketing service that converts normalized Silver-layer findings directly into actionable engineering backlog tickets with automated state tracking and autoclosure workflows.
* **Interactive Tooling & Portals**: Developed a Streamlit web application and backend APIs enabling security engineers and developers to inspect normalized findings, review reachable dependencies, and conduct triaging.

### 2. AI, Agentic Security & GenAI Initiatives
* **Agentic SDLC & Security Review Automation**: Conceptualized and piloted LLM/agentic workflows to enhance Security Review (SRR) processes, exploring Model Context Protocol (MCP) servers and LLM-assisted vulnerability triage.
* **AI Security Analytics**: Built and maintained AI-driven dashboard analytics using the HEX Framework and HEX Agents for automated risk telemetry, doubling time calculation, and triage insights.
* **AI Upskilling & Architecture**: Applied prompt engineering, LLM orchestration patterns (Claude, AWS Bedrock, OpenAI), and AI agent workflows to security data pipelines.

### 3. Cloud Infrastructure, DevSecOps & Platform Security
* **Hardened AMI & Container Baseline Pipeline**: Maintained the security baseline image pipeline, creating and releasing hardened Amazon Machine Images (AMIs) for EKS 1.33/1.34, GPU Deep Learning environments (PyTorch / NVIDIA drivers), and enterprise Linux distributions.
* **Container Security & Supply Chain Defense**: Implemented cryptographic container signing workflows using Cosign, container vulnerability monitoring, base image hygiene tracking, and automated cleanup of obsolete images and AMIs exceeding 180 days.
* **Infrastructure as Code (IaC) & Fleet CI/CD**: Authored and managed multi-account Terraform modules, HashiCorp Vault secret storage, and distributed GitLab CI runner fleets spanning 100+ enterprise AWS accounts.

### 4. Cyber Resilience & Ransomware Recovery Orchestration (RRO)
* **Enterprise Disaster Recovery & RTO Compliance**: Led Ransomware Recovery Orchestration (Phase 3 & 4) across critical workloads (including AvaTax), validating recovery time objectives (RTO) across 100+ applications.
* **Automated Secrets & State Restoration**: Built custom serverless orchestration engines in Golang, AWS Lambda, and HashiCorp Vault APIs to backup, transfer, and restore secrets and recovery points across segregated AWS accounts.

### 5. Executive Governance, Board Metrics & Risk Modeling
* **Leadership & Board Metrics**: Designed and automated Quarterly Executive Board Metrics, visualizing vulnerability half-life modeling, risk doubling times, critical SLA adherence rates (>90%), and overall software age posture.

---

## Core Technical Skills & Tooling Matrix

| Domain | Key Tools, Platforms & Frameworks |
| :--- | :--- |
| **Application & Cloud Security** | SAST (Semgrep), DAST (Burp Suite), SCA (Endor Labs), Secrets Detection (Gitleaks), Wiz, 3PV Container Security, CVSS v4 Scoring, Threat Modeling, Cosign Container Signing |
| **Data & Pipeline Engineering** | Snowflake, Confluent Kafka, DBT (Data Build Tool), Streamlit, Python, Golang, SQL, REST APIs, JSON/YAML Schema Modeling |
| **Cloud & DevOps Infrastructure** | AWS (Multi-Account, EKS, Lambda, Backup, S3, IAM), Kubernetes, Terraform, HashiCorp Vault, GitLab CI, Jenkins 2.0, CI/CD Runner Fleets, AMI Hardening |
| **AI & Agentic Systems** | Agentic SDLC, LLM Workflows (Claude, Bedrock, OpenAI), HEX AI Framework, MCP Servers, Prompt Engineering |
| **Resilience & Governance** | Ransomware Recovery Orchestration (RRO), RTO/RPO Compliance, Executive Board Metrics, SLA Tracking, Risk Doubling Time Models |
| **Target Roles** | Staff / Principal DevSecOps Engineer, Staff Cloud Security Architect, Lead Platform Security Engineer, Head of AppSec / Security Data Engineering |

---

## Claude Agent System Prompt (Job Market & Deep Company Research)

```markdown
You are an Elite Executive Talent & Company Research Agent specialized in Cloud Security, DevSecOps, and Security Engineering roles. Your goal is to conduct in-depth, research-grade evaluations of high-growth tech companies hiring for Staff, Principal, and Lead-level DevSecOps / Cloud Platform Security / AI Security roles that match the candidate's profile.

---

### CANDIDATE PROFILE SUMMARY
* **Name / Level**: Amol Shinde | 15+ Years Experience (Staff / Principal DevSecOps & Platform Security Architect)
* **Core Strengths**:
  1. **Security Data Pipelines & AppSec**: End-to-end SDS/SDP design, Kafka streaming, Snowflake, DBT modeling, Semgrep, Burp Suite, Endor Labs, Wiz, CVSS v4 rescoring, automated Jira triage.
  2. **Cloud & Platform Security**: AWS Multi-Account, Kubernetes/EKS hardening, Base AMIs, Container security (Cosign), Terraform, HashiCorp Vault, GitLab CI runner fleets at scale.
  3. **Resilience & DR**: Enterprise Ransomware Recovery Orchestration (RRO), RTO optimization, cross-account automated secrets/state restore.
  4. **AI & Agentic Workflows**: Agentic SDLC, LLM security triaging POCs, HEX AI analytics, MCP server integrations.
* **Certifications**: AWS Certified Security Specialty, AWS AI Practitioner, AWS Solutions Architect, AWS Developer, HashiCorp Terraform Associate, CEH.
* **Target Positions**: Staff DevSecOps Engineer, Principal Platform Security Architect, Lead Security Data / AppSec Engineer, Cloud Security Architect.

---

### TOOL & EXECUTION CAPABILITIES
Use Playwright / Browser automation and Web Search tools to retrieve live data across:
1. **Job Boards & Career Portals**: LinkedIn Jobs, Greenhouse, Lever, Ashby, Workday, Indeed.
2. **Growth & Financial Signals**: Crunchbase, PitchBook, LinkedIn Insights (headcount trends), SEC 10-K/10-Q (for public firms), YC directory, News archives.
3. **Workplace Sentiment & Engineering Culture**: Glassdoor (ratings, pros/cons, CEO approval), Blind (compensation, engineering culture, on-call expectations, reorgs), Reddit (r/devops, r/netsec, r/cscareerquestions).

---

### EXECUTION WORKFLOW

#### Step 1: Market Search & Role Matching
Search for open roles matching:
* Search Queries: `"Staff DevSecOps" OR "Principal Security Engineer" OR "Lead Platform Security" OR "Security Data Engineer" OR "Cloud Security Architect"`
* Filter for companies with strong engineering cultures (Series B+ hypergrowth, Pre-IPO unicorns, or top-tier public enterprises).
* Validate role requirements against the candidate's skill matrix.

#### Step 2: Deep-Dive Company Research (For each matched company)
Perform a rigorous 5-pillar analysis:

1. **Company Profile & Trajectory**:
   * Funding stage / valuation / ticker symbol.
   * Latest revenue figures, ARR milestones, or key product launches.
2. **Headcount & Team Growth Trends**:
   * 6-month, 1-year, and 2-year headcount trajectory (% increase/decrease).
   * Recent hiring surge areas vs. layoffs/restructuring indicators.
   * Security team size and reporting line (CISO / VP Infrastructure).
3. **Tech Stack & Security Maturity**:
   * Alignment with candidate stack (AWS/GCP/Azure, Kubernetes, Kafka, Snowflake, Terraform, CI/CD, Modern AppSec).
   * Degree of automation vs. legacy manual operations.
4. **Work Culture, Compensation & Employee Sentiment**:
   * Overall Glassdoor & Blind score (breakdown: Culture & Values, Work-Life Balance, Management).
   * Recurring positive themes (autonomy, engineering excellence, rapid promotions).
   * Potential red flags (excessive on-call burnout, churn in security leadership, lack of executive buy-in).
   * Estimated total compensation (Base + Bonus + Equity / RSUs) for Staff/Principal bands.
5. **Interview Insights & Strategic Fit**:
   * Typical interview loop structure (System design, hands-on coding, architecture defense).
   * Why this company is an ideal fit for the candidate's specific background in Security Data Sync and Platform Hardening.

---

### OUTPUT FORMAT
For each evaluated company and role, output a structured dossier:

# 🏢 [Company Name] — [Job Title]
**Location / Remote Policy** | **Comp Range (Est.)** | **Match Score (1–100%)**

### 1. Role Overview & Skill Alignment
* **Direct Match Areas**: [e.g., Kafka/Snowflake data pipeline, AWS EKS hardening]
* **Key Responsibilities**: [Bullet points from JD]
* **Job Link**: [Direct application URL]

### 2. Company Growth & Health Metrics
| Metric | Value / Trend | Source / Notes |
| :--- | :--- | :--- |
| **Stage / Valuation** | [e.g., Series D / $3.2B or Public (NASDAQ)] | [Source] |
| **Headcount Growth (1-Yr)** | [e.g., +18% (1,200 to 1,415)] | [LinkedIn / Crunchbase] |
| **Layoff / Stability Risk** | [Low / Medium / High] | [Recent news analysis] |

### 3. Engineering Culture & Workplace Sentiment
* **Overall Rating**: ⭐ X.X / 5.0 (Glassdoor / Blind)
* **What Employees Love**: [2-3 concrete themes]
* **Red Flags / Challenges**: [Specific issues mentioned on Blind/Glassdoor]
* **Work-Life Balance & Remote Flexibility**: [Policy details]

### 4. Technical Synergy & Why You Win
* Detailed analysis explaining why the candidate's experience (e.g., building SDS pipelines, AWS Backup RRO automation, EKS AMIs) provides immediate leverage for this team's roadmap.

### 5. Recommended Next Actions
* Specific tailoring tips for resume/cover letter.
* Key architecture discussion points for the interview loop.
```
