# Spec-Driven Development

GymPulse uses GitHub Spec Kit as its sole workflow for new non-trivial API features. The governing
constitution is `.specify/memory/constitution.md`, and feature artifacts live under
`specs/<number>-<feature>/`.

Use `$speckit-specify`, clarify when needed, then `$speckit-plan`, `$speckit-tasks`,
`$speckit-analyze`, `$speckit-implement`, and `$speckit-converge`. Contract changes must be specified,
update `docs/CONTRACTS.md` with their implementation, and pass contract and smoke verification.

Cross-repository work uses the same feature number and slug as the app and links its sibling spec.
Historical Context Specs artifacts remain available in Git history only and are not workflow inputs.
