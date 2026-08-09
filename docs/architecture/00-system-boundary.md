# System Boundary Route

- Status: Active index
- Authority: [`../02-system-context.md`](../02-system-context.md)

RelayDock is neither one deployment artifact nor a general-purpose model proxy. The Desktop Runtime, Managed Gateway, and Control Console are connected within one product, but they do not share trust or deployment boundaries.

## Boundary Sources

- System structure and trust boundaries: [`../02-system-context.md`](../02-system-context.md)
- Code and package responsibilities: [`../03-repository-map.md`](../03-repository-map.md)
- Request state and failure semantics: [`../04-request-lifecycle.md`](../04-request-lifecycle.md)
- External communication and secret threats: [`../11-security-threat-model.md`](../11-security-threat-model.md)
- Deployment and artifact boundaries: [`../15-build-and-release.md`](../15-build-and-release.md)

When adding a component or authoritative data owner, update the owning authority document before this route page.
