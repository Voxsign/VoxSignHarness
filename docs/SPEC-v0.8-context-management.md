# SPEC v0.8 — Context Management (C)

Context management governs what the model sees: cleaned input, the personal
dictionary and memory, conversation slots, and punctuation/negation
normalisation of spoken commands. The goal is that ambiguous or negated
speech becomes a clarification, never an executed side effect.

## Contract

- Raw spoken input is cleaned deterministically; punctuation is idempotent
  and never inserted inside code spans.
- The personal dictionary persists across reloads, corrects Chinese
  substrings and English token boundaries, and renders nothing when empty.
- A zero-information utterance trips the slot gate and produces an ask; an
  informative utterance does not.
- Explicit negation ("don't", "不要/别") routes to an ask or a cancel rather
  than an execution, and negation outranks conditional and meta readings.
- Multi-action and continuation utterances are classified into slots and
  routed to the right task without merging distinct objects.

## Pinned validators

- `TestAddTermPersistAndReload`
- `TestCorrectChineseSubstring`
- `TestCorrectEnglishTokenBoundary`
- `TestRenderNonEmpty`
- `TestClean`
- `TestPunctuateIdempotent`
- `TestPunctuateNoPunctuationForCode`
- `TestSlotGateAsksOnZeroInformation`
- `TestSlotGateDoesNotFireOnInformativeText`
- `TestNegationBecomesAskNotExecute`
- `TestNegationMarkerPrecedence`
- `TestR10ContinuationAndProgressRouting`
