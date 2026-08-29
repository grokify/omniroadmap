# PRD — COMPASS-RICE Prioritization — MoSCoW + normalized cross-profile ranking, scoring CLI, and DashForge dashboards

## Problem

omniroadmap's ranking pipeline (prism-roadmap `RankingPolicy`: MoSCoW tier first, RICE score within tier) uses a single-scale RICE where Reach is one 0..1 fraction of "the relevant customer population." The raw metrics behind that fraction are not comparable across opportunity types — a customer feature's affected-ARR percentage, a platform investment's team leverage, and a risk mitigation's exposure reduction are different quantities. Ranking them on one naive Reach scale produces orderings nobody trusts, which pushes prioritization back into opinion.

## Vision

Adopt COMPASS-RICE (`github.com/ProductBuildersHQ/compass-rice`) as the RICE score source: every opportunity is scored under exactly one of six investment-thesis profiles (Customer, Platform, Market Expansion, Operational Efficiency, Supportability, Risk), each with a domain-specific evidence model whose deterministic normalizer produces the same canonical `rice.Normalized` shape (Reach 0–100 band, standard Impact/Confidence multipliers, Effort person-days). Scores are then comparable portfolio-wide, so MoSCoW + COMPASS-RICE can rank everything — even when the raw metrics are not.

Humans and agents score initiatives through the CLI (write path); everyone reviews and validates through DashForge dashboards (read path).

## Users

- **PM (human)**: confirms profile assignments, reviews flagged assessments, sets MoSCoW, validates normalized scores against raw evidence in per-profile dashboard views.
- **LLM judge (agent)**: proposes profile assignments and produces evidence-backed `judge.Output` documents; never emits a score directly.
- **Portfolio reviewers**: consume the ranked views and opportunity reports.

## Two-phase scoring (core workflow)

1. **LLM-as-a-Judge proposes**: the judge selects a profile (primary investment thesis), fills the profile's typed Evidence struct, and cites verifiable claims. Confidence is derived deterministically from verified claims — never self-reported.
2. **Human PM confirms**: the profile assignment must be PM-confirmed, and any assessment flagged `NeedsHumanReview` must be human-accepted, before its score enters the ranking.

## Requirements

- FR1: Exactly one confirmed profile generates an opportunity's canonical RICE score (compass-rice PRD D5 — no score shopping). Secondary theses recorded as context only.
- FR2: Opportunities without a confirmed profile + reviewed COMPASS assessment are **excluded from ranking** with an explicit reason — never silently dropped, never scored via the legacy 0..1 ladder scale (mixed regimes are numerically incoherent).
- FR3: MoSCoW gating, tie bands, and rank overrides are unchanged — compass-rice supplies the score, prism-roadmap owns the ranking.
- FR4: A judge's evidence source counts must be consistent with its verified claims (confidence integrity check at ingest); inconsistent outputs are rejected with repair prompts.
- FR5: Three prioritization views plus a portfolio overview, all defined as DashForge DashboardIR: all-items by COMPASS-RICE, MoSCoW+COMPASS-RICE, and per-profile (raw evidence beside normalized band/method/score for human validation). Non-normalized inputs are always inspectable from cross-profile views — never the score alone.
- FR6: Cobra CLI write path: query initiatives and their profiles/scores, import LLM judge output, enter human-attested evidence, manage profile assignments, view/set MoSCoW — all thin wrappers over library packages, `--json` output for agents.

## Non-goals

- No LLM runner inside omniroadmap — judge execution stays external; omniroadmap ingests `judge.Output` documents.
- No multi-thesis scoring (a v1 limitation inherited from compass-rice: multi-thesis initiatives may be undervalued until a future profile version accounts for secondary value).
- No removal of the legacy ladder RICE API from prism-roadmap (it remains for existing consumers; omniroadmap's pipeline just never falls back to it).
- No replacement of the existing hand-rolled items table UI — it stays, but is no longer the dashboard path.

## Success criteria

- A portfolio containing customer, platform, and risk opportunities produces a single ranked list whose within-tier ordering derives entirely from comparable COMPASS-RICE scores.
- An agent can score an initiative end-to-end via CLI (propose profile → import judge output) and a PM can gate it (confirm profile, accept flagged assessments) without touching the database directly.
- A PM can open the per-profile dashboard and validate any normalized score against the raw evidence that produced it.
