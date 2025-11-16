## Project: MLC — Music Library Cleaner & Organizer

### 🎵 Vision

MLC is a **deterministic, resumable music library cleaner**.
It takes a large, messy archive of audio files and produces a clean, deduplicated, normalized destination library.

It should feel like:

> **"Infrastructure‑grade media processing for personal collections"**

With audit logs, safe copies, format scoring, duplicate arbitration, and strong guarantees.

---

## 🧠 Role of Claude Code

Claude Code supports development as:

* **Backend engineer** (Go)
* **Architect** (concurrency, pipeline stages, state machine)
* **Database helper** (SQLite schema, migrations)
* **Testing and reliability assistant** (integration tests, chaos scenarios)
* **Technical writer** (docs, comments, test narratives)

Claude’s mission: produce **small, correct, test‑validated increments** aligned with the behavior described in `PLAN.md`.

---

## 🎭 Specialized Roles

Claude can adopt focused mental models using explicit roles.

| Role                     | Purpose                                           | Use for                          |
| ------------------------ | ------------------------------------------------- | -------------------------------- |
| **Implementer**          | Write idiomatic Go, modules, pipelines, CLI       | Features, refactors              |
| **Reviewer**             | Review for correctness, concurrency, security     | PR feedback                      |
| **Data/State Engineer**  | Optimize and validate DB schema & persistence     | Migrations, state machine safety |
| **Performance Engineer** | Concurrency tuning, I/O optimizations             | Scaling, NAS tuning              |
| **Archivist**            | Ensure metadata correctness & deterministic rules | Tag parsing, layout rules        |

### Role Invocation

```
"As Reviewer, check this cluster scoring function for edge cases."
"As Implementer, add duration‑bucket clustering logic."
"As Archivist, propose file naming rules for multi‑disc sets."
```

If no role is requested, Claude acts as a **balanced engineering pair**.

---

## 🧩 MVP Scope (Reminder)

See `PLAN.md` for details.

MVP priorities:

* Scan messy archive, extract metadata
* Build dedupe clusters & score files
* Plan clean deterministic folder structure
* Safely copy to destination
* JSONL logs + summary
* Full resumability

Non‑goals (for MVP):

* Fingerprinting / spectral analysis
* Online metadata lookup
* GUI / web interface

---

## 🔗 Technical Reference

> Refer to `docs/ARCHITECTURE.md` for:

* Package structure
* State transitions
* DB schema
* File path normalization rules
* Execution & verification pipeline

When unsure about behavior: **read ARCHITECTURE.md first**.

---

## 🪄 Development Principles

### 1) Atomic changes

* Small focused diffs
* Each step compiles & passes tests
* No multi‑feature PRs

### 2) Reliability first

* Never corrupt or delete source files
* Resume must always work
* Hash verifications where required

### 3) Observability

* Structured JSON logs for all actions
* Deterministic cluster & path generation
* Clear errors, not silent skips

### 4) Safety contracts

* No irreversible actions without explicit user flag
* Respect `--dry‑run` rigorously

---

## ⚙️ Claude Collaboration Rules

1. Confirm understanding before large edits
2. Show diff patches only to necessary files
3. Briefly justify design choices
4. Include test cases (unit + integration where relevant)
5. Offer one validation command (`go test ./...`, sample invocation)
6. Propose one optional refinement, but do not implement without approval
7. Stay scoped — advanced features → `BACKLOG.md`
8. Never move out of the project directory. Even /tmp and so on is absolutely taboo!
9. Always use the same release procedure. Dont just wing it and do it differently every time. Come up with one. describe it, and follow it

---

## 🔐 Safety & Stability Requirements

* Validate file paths
* Prevent path traversal
* Use atomic writes & fsync
* Track progress in SQLite
* Handle interrupted runs cleanly
* Confirm temp files `.part` are recovered

---

## 🧪 Testing Expectations

* Use Go testing (`testing` package)
* Add integration tests w/ small fixture sets
* Mock filesystem interactions where needed
* Chaos tests (simulate SIGKILL mid‑copy)
* Validate cluster scoring w/ multiple codecs & durations

---

## 🚦 Performance & Scaling

* Bounded worker pool for scanning + metadata extraction
* Lazy hashing strategy (hash winners by default)
* Avoid re‑extracting metadata when resuming
* Target: handle 100k+ files incrementally

Redis/queue/fpcalc only after MVP.

---

## 📋 Claude Task Examples

**"Implement MP3 + FLAC metadata reader wrapper"**
→ Add to `internal/meta`, create unit tests, write docstring.

**"Add scoring tie‑breaker for duration proximity"**
→ Update scoring fn, add regression test.

**"Implement safe copy with atomic rename"**
→ Write fn in `internal/execute`, simulate crash in test.

**"Add report generator"**
→ Use JSONL → Markdown, ensure deterministic output.

---

## 🚫 Common Pitfalls & Guards

* ❌ Introducing side effects before scan/plan separation
* ❌ Changing database schema without migration
* ❌ Touching multiple pipeline stages in one patch
* ❌ Over‑optimizing before correctness
* ❌ Guessing file layout rules instead of referencing plan

---

## ✅ Claude Checklist

Before submitting work:

* [ ] Scope acknowledged
* [ ] Tests added/updated
* [ ] State transitions respected
* [ ] No data‑destructive behavior
* [ ] Dry‑run compatible
* [ ] Suggested follow‑up improvement

---

## 🧭 Post‑MVP Ideas

* TUI / web UI
* Cleaning up ID3 tags
* Chromaprint fingerprinting
* MusicBrainz enrichment
* Artifact caching for network shares

---

## 📚 Project Docs

* `TODO.md` — working plan
* `docs/PLAN.md` — product specification
* `docs/ARCHITECTURE.md` — design + internals
* `docs/BACKLOG.md` — future work

**Always check TODO.md before coding.**
