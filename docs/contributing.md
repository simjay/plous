# Contributing

[Documentation](README.md) · [Development](development.md)

Plous is early in development. Small fixes, clear bug reports, and corrections to the guides are useful. Discuss changes to navigation, shortcuts, storage, or product scope before building a large feature.

## Report a problem

Open a [GitHub issue](https://github.com/simjay/plous/issues) with:

- The Plous version, operating system, and terminal.
- Steps to reproduce, expected behavior, and what happened.
- Relevant context: the view, resource kind, and error text.
- A screenshot or short recording if the problem is visual.

Remove tokens, API keys, private object values, and personal data. Do not attach your profile directory or kubeconfig. For Kubernetes problems, include the server version and whether the same operation works through another client.

Check [Troubleshooting](troubleshooting.md) first. A permission denial or absent API may be expected behavior rather than an application bug.

## Submit a change

1. Keep the change focused on one problem.
2. Follow the existing package boundaries and reuse the shared viewers.
3. Add regression coverage for changed behavior.
4. Update the affected guide and keymap in the same change.
5. Run `make check` for application changes, `make dist` for packaging changes, or `make docs-check` for documentation changes.

Describe the problem, resulting behavior, and checks in the pull request. State which checks used fixtures and which used a live service. Keep generated files, credentials, local data, and local dot-directories out of the change; the root `.github/` holds workflows.

Do not bump the application version or add a release tag as part of an unrelated fix. The version source is `cmd/plous/VERSION`.

## Write useful docs

Describe what the current build does. Use the actual label or key, explain the result, and stop there. Put detailed bindings in [Keyboard shortcuts](keymap.md) and link to them from task guides. Avoid repeating whole procedures across pages.

See [Maintain these docs](documentation.md) for previews and publishing, and [Development](development.md) for code structure and tests.

## License

Plous uses the [Apache License 2.0](../LICENSE).
