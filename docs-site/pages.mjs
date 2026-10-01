export const navigation = [
  { label: 'Start here', items: [
    ['README', 'Introduction'],
    ['installation', 'Installation'],
    ['getting-started', 'First workspace'],
  ] },
  { label: 'Using Plous', items: [
    ['workspaces', 'Workspaces and connections'],
    ['navigation', 'Interface and navigation'],
    ['activity', 'Activity and history'],
    ['ai-chat', 'AI assistant'],
  ] },
  { label: 'Integrations', items: [
    ['integrations', 'Tools and capabilities'],
    { label: 'Kubernetes', collapsed: true, items: [
      ['kubernetes', 'Connect a cluster'],
      ['kubernetes-explore', 'Explore resources'],
      ['logs', 'Read logs'],
      ['exec', 'Container terminals'],
      ['kubernetes-actions', 'Resource actions'],
      ['editor', 'Edit and review YAML'],
      ['resources', 'Resource support'],
      ['playground', 'Playground'],
    ] },
  ] },
  { label: 'Reference', items: [
    ['keymap', 'Keyboard shortcuts'],
    ['configuration', 'Configuration'],
    ['data-and-privacy', 'Data and privacy'],
    ['troubleshooting', 'Troubleshooting'],
  ] },
  { label: 'Contribute', items: [
    ['contributing', 'Contributing'],
    ['development', 'Development'],
    ['documentation', 'Maintain these docs'],
  ] },
];

const flatten = (items) => items.flatMap(item => Array.isArray(item) ? [item] : flatten(item.items));
export const pages = flatten(navigation);
export const slug = (name) => name === 'README' ? '' : name;
export const sidebar = (items = navigation) => items.map(item => Array.isArray(item)
  ? { slug: slug(item[0]), label: item[1] }
  : { ...item, items: sidebar(item.items) });
export const base = '/plous/';
export const repository = 'https://github.com/simjay/plous';
