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
