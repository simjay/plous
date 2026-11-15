function mergedGuide(slug, route, defaultSection, sections) {
  const anchors = Object.fromEntries(Object.entries(sections).flatMap(([section, names]) =>
    names.map((name) => [name, route + '#' + section])));
  return {slug, destination: route + '#' + defaultSection, anchors};
}

const explore = 'docs/guides/navigation/';
const assistant = 'docs/guides/ai-chat/';
const mcpSetup = 'docs/mcp/setup/';

// Keep published links usable while each guide has one canonical source.
export const legacyRoutes = [
  {
    slug: 'getting-started/workflows',
    destination: 'docs/',
    anchors: {
      'terminal-ui': 'docs/getting-started/',
      'mcp-server': 'docs/mcp/setup/',
      'agent-skills': 'docs/agent-skills/plous-debug/#install',
    },
  },
  {
    slug: 'agent-skills',
    destination: 'docs/agent-skills/plous-debug/',
    anchors: {
      'available-skills': 'docs/agent-skills/plous-debug/#install',
      'skills-and-mcp': 'docs/agent-skills/plous-debug/#what-setup-installs',
    },
  },
  {
    slug: 'mcp',
    destination: 'docs/mcp/setup/',
    anchors: {
      'connect-your-client': 'docs/mcp/setup/#configure-your-client',
      'choose-the-connection-mode': 'docs/#choose-your-workflow',
    },
  },
  {
    slug: 'guides/debug-with-your-agent',
    destination: 'docs/agent-skills/plous-debug/',
    anchors: {},
  },
  mergedGuide('guides/workspaces', explore, 'workspaces-and-connections', {
    'review-activity': ['activity-and-revisions'],
  }),
  mergedGuide('guides/overview', explore, 'read-overview', {}),
  mergedGuide('guides/activity', explore, 'review-activity', {}),
  mergedGuide('guides/plous-mcp', assistant, 'run-claude-code-or-codex', {
    'investigate-a-workload': ['try-an-investigation', 'available-tools', 'scope-access-and-limits'],
    'inspect-a-replys-sources': ['inspect-saved-evidence'],
  }),
  mergedGuide('mcp/scope', mcpSetup, 'saved-evidence', {
    'target-and-evidence-options': ['captured-access'],
  }),
  mergedGuide('mcp/troubleshooting', mcpSetup, 'troubleshoot-the-connection', {}),
  {
    slug: 'integrations/gitops',
    destination: 'docs/integrations/',
    anchors: {
      'open-the-view': 'docs/integrations/argo/#open-the-view',
      'argo-applications': 'docs/integrations/argo/#argo-applications',
      'argo-applicationsets-and-projects': 'docs/integrations/argo/#argo-applicationsets-and-projects',
      'flux-kustomizations-and-helmreleases': 'docs/integrations/flux/#flux-kustomizations-and-helmreleases',
      'flux-sources': 'docs/integrations/flux/#flux-sources',
      'limits': 'docs/integrations/argo/#limits',
    },
  },
];
