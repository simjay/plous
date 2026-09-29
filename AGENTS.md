# Working on Plous

Jae Sim governs product, design, and technical decisions. Record material choices and changed direction in `.decisions/`; distinguish explicit owner decisions from proposals.

Work directly on `main` unless the owner requests another branch. The owner handles pushes; keep changes local and do not push unless explicitly asked.

Directories whose names start with `.` are local internals and must never be committed or pushed. The owner explicitly permits only the root `.github/` directory for CI and release workflows. Preserve local internal files, including `.decisions/`, but keep them untracked. Before pushing, check the proposed commits as well as the current tree for prohibited paths; do not push old local history containing internals.

Current scope is the working workspace layer in `.decisions/0021-workspace-layer-implementation-plan.md`, authorized by the owner's "build" request. It includes workspace creation/settings, local kubecontext discovery, per-tool configuration references, the workspace tree, four main modes, configuration Activity, and local chat drafts. Keep the code and repository small. Live resource browsers, deployment execution, model calls, and agent execution await a later request.

Commands belong to the focused panel. Main-panel :1–:4 select modes; the tree has no numeric mode shortcuts and resource bindings must be designed per tool. Text and paste in forms/chat never execute navigation commands. Tab/Shift+Tab switch panels, including during forms. Test with temporary storage or --data-dir; do not alter the owner's ~/.plous or kubeconfig during verification.

Every command must have an explicit short alias and a full-word spelling. Keep aliases unambiguous within their scope. Main-panel colon input opens a bordered command box at the bottom of that panel, with the current content above it.

Keep README.md and docs/ user-facing and aligned with implemented behavior. The main panel uses four evenly distributed tabs with right-aligned shortcuts, the assistant composer stays at the bottom, and the application has one footer line. Do not restore a Main heading or add extra footer/status rows.

Plous is version `0.0.1`, an early development build, not ready to ship. The single version source is `cmd/plous/VERSION`. Do not change the major or minor version without explicit owner direction. Do not automatically bump versions or create release tags. Dependency and Go toolchain versions are independent of the Plous version.

The initial TUI concepts were removed at the owner's request. Earlier PRD and technical documents describe deferred future ideas, not implementation instructions. Discuss future functionality and design one step at a time.

Run `make check` for application changes and `make dist` for packaging changes. Keep generated files out of source control. Document a hosted workflow or release as verified only after it actually succeeds.
