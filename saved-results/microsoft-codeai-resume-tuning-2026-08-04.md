# Microsoft Code|AI Resume Tuning

Date: 2026-08-04

Purpose: Target the current one-page resume at Microsoft CoreAI Code|AI roles while preserving factual evidence.

Source: `/Users/aadivyar/Desktop/swe_base.pdf` (latest one-page PDF created 2026-08-04 at 12:02).

User-confirmed evidence: retrieval-augmented generation was used in Fermi's Teach-Me-Anything flow.

## Phrase-level replacements

| Previous phrase | New phrase |
| --- | --- |
| `Expanded the tutoring system from a fixed topic catalog to open-ended “learn anything,” with explanations grounded in high-school prerequisites` | `Expanded the tutoring system from a fixed topic catalog to the open-ended Teach-Me-Anything flow; used retrieval-augmented generation (RAG) in that flow, with explanations grounded in high-school prerequisites` |
| `Cut time-to-diagnose failing eval cases from ~25 min to under ~3 min by shipping a tutoring-system test visualizer for step-level traces` | `Cut time to diagnose failing evaluation cases from ~25 minutes to under ~3 minutes by shipping a tutoring-system test visualizer for step-level traces` |
| `Built a customer-conversation recording and live transcription system with sub-1s end-to-end latency so founders capture interviews without losing detail to notes` | `Engineered a low-latency streaming transcription system with sub-1-second end-to-end latency so founders could capture interviews without losing detail to notes` |
| `Built a Graph RAG layer over those conversations so founders can query customer data and simulate marketing reactions in under ~2 min instead of hours of re-listening` | `Built a Graph RAG layer over customer conversations so founders could query customer data and simulate marketing reactions in under ~2 minutes instead of hours of re-listening` |
| `Drove weekly programming for a 300+ member org over a 14-week semester, teaching voice agents and multi-agent systems with consistent standing-room interest` | `Led weekly programming for a 300+ member organization over a 14-week semester, teaching voice agents and multi-agent systems with consistent standing-room interest` |
| `Built a voice clinical-intake prototype with Carle collaborators and passed 100+ simulated patient cases before faculty review` | `Built and evaluated a voice clinical-intake prototype with Carle collaborators, passing 100+ simulated patient cases before faculty review` |

## Skills line to add

Add this line above `Languages`:

`AI/ML: Retrieval-augmented generation (RAG), LLM evaluation, multi-agent systems, low-latency AI systems`

If still current, also restore `PyTorch`, `CUDA`, and `OpenAI APIs` to the skills section; they appeared in the prior base resume but are not present in the new one-page version.

## Keywords deliberately not added

The current evidence does not yet support claiming code completion/editing, program-analysis models, codebase-aware assistance, efficient inference algorithms at scale, or GitHub Copilot-specific benchmarks. Add those only with a concrete project, paper, or measured system result.
