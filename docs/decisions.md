# Decisions

## Synthetic-only pipeline

The monitor accepts only the existing checked-in canary suite. This keeps
privacy boundaries enforceable and prevents production prompts from becoming
baseline data.

## Hash-based style features

Fixed hashed trigrams and word unigrams avoid external ML dependencies and
retain only comparative structure. Collisions and lack of semantic meaning
are documented limitations.

## Explicit inconclusive verdicts

Attribution thresholds and margins are configurable. When evidence overlaps
or is insufficient, the engine records `INCONCLUSIVE` and schedules a
discriminating follow-up instead of presenting an inference as fact.

## SQLite migrations

Persistence uses small versioned migrations and direct SQL through
`modernc.org/sqlite`, matching the backend-only and single-dependency
constraint.
