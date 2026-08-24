# Effort and RICE Evaluation

OmniRoadmap can use LLM-as-Judge evaluation as an input to prioritization. The first useful rubric is an effort/RICE estimator:

- `effort`: estimated implementation effort from codebase evidence
- `reach`: estimated affected users/customers/workflows
- `impact`: estimated business or technical value
- `confidence`: confidence in the evidence and estimate

The rubric definition is in:

```text
rubrics/roadmap_effort_rice.yaml
```

## Inputs

The judge should receive a bounded evidence packet, not an entire repository dump:

- roadmap item title, description, acceptance criteria, and relevant custom fields
- ORQL-selected columns such as `custom.problem_statement`, `custom.launch_tier`, or customer fields
- codebase summary: touched packages, file counts, languages, tests, migrations, generated code
- architecture summary: whether work extends an existing path or introduces a new subsystem
- dependency summary: upstream teams, external APIs, vendors, permissions, release trains
- ownership map: path patterns to team owners
- existing telemetry, customer count, support volume, ACV, or usage data when available

## Output

The evaluator should return a `structured-evaluation/rubric.Rubric` report with one category result per RICE dimension. OmniRoadmap can then store normalized estimates on the roadmap item:

```json
{
  "effort": 3,
  "reach": 4,
  "impact": 4,
  "confidence": 3,
  "evidence": {
    "effort": ["touches entitlement APIs and workflow UI", "requires one migration"],
    "confidence": ["ownership map missing for reporting package"]
  }
}
```

For RICE, effort is inverted during scoring: higher effort lowers the final RICE score.

## Prioritization

The `moscow_rice` algorithm can use these estimates:

1. Exclude `wont_have`.
2. Rank `must_have` by RICE.
3. Rank `should_have` by RICE.
4. Rank `could_have` by RICE.
5. Route low-confidence estimates to human review.

This keeps MoSCoW as the product commitment tier and RICE as the within-tier sorter.

## Guardrails

LLM estimates are advisory. They should not replace engineering planning for committed work.

- Low-confidence results need human review.
- The judge must cite evidence.
- Missing code ownership lowers confidence.
- New architecture, infrastructure, migrations, or cross-team dependencies increase effort.
- The same rubric can be run by multiple judges and compared using structured-evaluation inter-rater reliability.
