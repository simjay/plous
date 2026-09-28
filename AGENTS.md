# Working on Plous

Jae Sim governs product, design, and technical decisions. Record material choices and changed direction in `.decisions/`; distinguish explicit owner decisions from proposals.

Directories whose names start with `.` are local internals and must never be committed or pushed. The owner explicitly permits only the root `.github/` directory for CI and release workflows. Preserve local internal files, including `.decisions/`, but keep them untracked. Before pushing, check the proposed commits as well as the current tree for prohibited paths; do not push old local history containing internals.

Current scope is the minimal executable and build/distribution pipeline in `.decisions/0009-minimal-bootstrap.md`. The executable only prints `welcome to plous`. Keep the code and repository small. Do not add application architecture, UI, integrations, or agent features ahead of the next owner request.

The initial TUI concepts were removed at the owner's request. Earlier PRD and technical documents describe deferred future ideas, not implementation instructions. Discuss future functionality and design one step at a time.

Run `make check` for application changes and `make dist` for packaging changes. Keep generated files out of source control. Document a hosted workflow or release as verified only after it actually succeeds.
