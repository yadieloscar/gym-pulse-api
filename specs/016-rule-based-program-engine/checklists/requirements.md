# Requirements Checklist: Rule-Based Program Engine

- [x] Every functional requirement is testable and has an evidence row in quickstart.md.
- [x] The contract change is additive; an older app keeps working (R9).
- [x] Every catalog entry has a seed row in data-model.md (79 of 79).
- [x] Every table in research R3 names its source via spec 015 research "Goal guide".
- [x] Privacy: no field leaves the process on the engine path (FR-011).
- [x] Failure answers are documented: too-constrained → 422 `avoid`; unknown values → 422 on the
      field; model path without a key → falls back to rules with a warning.
- [x] Non-goals exclude progression, history adaptivity, free-text parsing, draft editing.
- [ ] Owner review of the seed mapping assumptions (bar-dependent entries marked `full_gym`;
      swings and carries accept `dumbbells`; swimming `full_gym`).
