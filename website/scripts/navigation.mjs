export const groups = [
  { label: 'Get started', items: [['Installation & updates','getting-started/installation']] },
  { label: 'Terminal UI', items: [
    ['Quickstart','getting-started'],
    ['Navigation & controls','guides/navigation'], ['Workspaces & connections','guides/workspaces'],
    ['Cluster overview','guides/overview'], ['Activity & history','guides/activity'],
    ['AI assistant','guides/ai-chat'], ['Claude Code & Codex inside Plous','guides/plous-mcp'],
    ['Investigate a restarting Pod','guides/investigate-restarting-pod'],
  ] },
  { label: 'Coding agents', items: [
    ['Set up plous-debug','agent-skills/plous-debug'], ['Connection troubleshooting','mcp/troubleshooting'],
    ['Context & saved evidence','mcp/scope'], ['Manual MCP setup','mcp/setup'], ['Active diagnostics','mcp/active-diagnostics'],
  ] },
  { label: 'Integrations', items: [['Capabilities & connection setup','integrations'],['Kubernetes','integrations/kubernetes'],['Helm','integrations/helm'],['Argo CD','integrations/argo'],['Flux','integrations/flux'],['Kubernetes playground','integrations/kubernetes/playground'],['GitOps playground','integrations/gitops/playground']] },
  { label: 'Reference', items: [['Keyboard shortcuts and commands','reference/keymap'],['Configuration','reference/configuration'],['Agent tools & permissions','reference/mcp'],['Data & privacy','reference/data-and-privacy'],['Troubleshooting','reference/troubleshooting']] },
  { label: 'Developers', items: [['Contributing','contributing'],['Architecture','contributing/#build-and-layout'],['AI implementation','contributing/ai-assistant'],['Testing & benchmarks','contributing/#run-checks'],['Releases','contributing/releases'],['Website maintenance','contributing/website']] },
];
