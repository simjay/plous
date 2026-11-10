// Keep published links usable while each guide has one canonical source.
export const legacyRoutes = [
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
