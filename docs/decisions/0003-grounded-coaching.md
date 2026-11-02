# 3. Grounded coaching: rules by default, LLM optional, both validated

**Status:** accepted (v0.1)

## Context

The product promise is "evidence over vibes", and LLM cost has to stay small.
Without guardrails an LLM invents rounds, blames the wrong player and turns
noise into advice.

## Decision

1. The coach gets **aggregates plus a few examples**, never the demo. For each
   party member and kind it sees the count, chances, rate, lobby rate and up to
   6 example findings with IDs. A whole match fits in about 5k tokens.
2. An issue **stands out** only if it happened at least 3 times *and* the count
   is at least one standard deviation above what a lobby-average player would
   produce with the same chances (binomial model). Both coaches may only pick
   issues that stand out. This replaced an earlier "at least one more mistake
   than average" rule, which let 63% vs. a 56% lobby rate through.
3. `Input.Validate` rejects advice that:
   - names someone outside the party;
   - has more than 3 focus areas;
   - picks an issue that does not stand out;
   - cites findings that don't exist, are about someone else, or are of a
     different kind.

   An invalid LLM reply gets one retry with the errors, then fails.
4. Two coaches implement one interface:
   - `Rules` (default): deterministic, free, offline. It is also the baseline an
     LLM has to beat.
   - `LLM`, with providers for Ollama, for any OpenAI-compatible API, and for
     the Codex CLI. Ollama goes through its native API because that lets us set
     the context size; its OpenAI-compatible endpoint is capped at 4k tokens. The
     Codex provider runs `codex exec` read-only in an empty directory. That lets a
     ChatGPT subscription be used through the official client: we do not call
     ChatGPT's private backend with its OAuth tokens, which is undocumented and
     can break at any time.

## Consequences

- With a local 30B model (Qwen3 on Ollama), the first prompt produced issues
  below the lobby rate and wrong comparisons. With the stands-out contract and
  the validator in place, its output passed validation and read well. The rules
  coach stays the default because it is predictable.
- Validation checks citations, not every sentence: a model could still describe
  a correct finding wrongly. Showing the evidence next to every claim lets the
  reader check it.
