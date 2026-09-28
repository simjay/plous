# Working on Plous

Jae Sim governs product, design, and technical decisions. Record material choices and changed direction in `.decisions/`; distinguish explicit owner decisions from proposals.

Directories whose names start with `.` are local internals and must never be committed or pushed. The owner explicitly permits only the root `.github/` directory for CI and release workflows. Preserve local internal files, including `.decisions/`, but keep them untracked. Before pushing, check the proposed commits as well as the current tree for prohibited paths; do not push old local history containing internals.

Current scope is the full-screen three-panel shell in `.decisions/0011-three-panel-shell.md`: a narrow left list, a center main view, and a right AI assistant chat placeholder. Keep the code and repository small. Do not add integrations, real chat, or agent execution ahead of the next owner request.

Plous is version `0.0.1`, an early development build, not ready to ship. The single version source is `cmd/plous/VERSION`. Do not change the major or minor version without explicit owner direction. Do not automatically bump versions or create release tags. Dependency and Go toolchain versions are independent of the Plous version.

The initial TUI concepts were removed at the owner's request. Earlier PRD and technical documents describe deferred future ideas, not implementation instructions. Discuss future functionality and design one step at a time.

Run `make check` for application changes and `make dist` for packaging changes. Keep generated files out of source control. Document a hosted workflow or release as verified only after it actually succeeds.
