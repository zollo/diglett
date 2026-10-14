# Contributing to Diglett

## Branching & merging

- Branch off `main` for every change (`feat/…`, `fix/…`, `ci/…`, `docs/…`).
- Open a PR into `main`. CI (gofmt, vet, tests, image build) must pass.
- PRs are **squash-merged**; the **PR title becomes the commit subject** on
  `main`, and the merged branch is deleted automatically.

## Conventional Commits

The **PR title must follow [Conventional Commits](https://www.conventionalcommits.org/)**,
because it becomes the squash commit that drives automated semantic versioning
on merge to `main`. A CI check (`PR Title`) enforces this.

```
<type>[optional scope][!]: <description>
```

**Types** and their effect on the next release version:

| Type | Example | Version bump |
|------|---------|--------------|
| `feat` | `feat: add DNSSEC validation to lookups` | **minor** (x.**Y**.0) |
| `fix` | `fix: handle truncated UDP responses` | **patch** (x.y.**Z**) |
| `feat!` / `fix!` / any `!` | `feat!: rename the /api/lookup response fields` | **major** (**X**.0.0) |
| `docs`, `chore`, `ci`, `build`, `refactor`, `perf`, `test`, `style`, `revert` | `docs: document resolver config` | patch (default) |

Mark a breaking change with `!` in the PR title. A `BREAKING CHANGE:` footer in
the PR body is not enough on its own: the squash commit keeps only the title.

### Guidelines
- Keep the subject imperative and lowercase, no trailing period
  (e.g. `fix: handle empty resolver list`, not `Fixed the bug.`).
- Use a scope when it helps: `feat(lookup): …`, `fix(ci): …`.

## Releases

On merge to `main`, CI computes the next `vX.Y.Z` from the PR title, tags it,
creates a GitHub release, builds, signs and attests the container image
(`latest`, `X.Y.Z`, `X.Y`, `sha-<short>`), and publishes the Helm chart with
version and appVersion `X.Y.Z` to `oci://ghcr.io/zollo/charts`. Don't bump
`charts/diglett/Chart.yaml` by hand. See the README's **Releases & versioning**
section.
