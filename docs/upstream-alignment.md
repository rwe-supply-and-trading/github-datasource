# Upstream alignment — 2026-11-21

The current integration incorporates two additional `grafana/github-datasource`
commits through [`5917ad1`](https://github.com/grafana/github-datasource/commit/5917ad1c84f6bfe8c22c6e49ea46a1632c837d94):

- [`7e74934`](https://github.com/grafana/github-datasource/commit/7e74934ec4f178bca4cc1aaaf424125c496516f6): refresh transitive frontend dependencies for security fixes and deduplicate the lockfile dependency tree.
- [`5917ad1`](https://github.com/grafana/github-datasource/commit/5917ad1c84f6bfe8c22c6e49ea46a1632c837d94): prepare upstream 2.11.0, enable private-package versioning without Changesets tags, and move already released changesets into the changelog.

The private version stays **2.11.0-rwe.1**, now based on released upstream
**2.11.0**. The complete upstream dependency graph is retained with RWE root
package identity and version. Conflicts in package metadata and the changelog
were resolved without replacing the RWE package name, repository, author or
private release sections. No Go or query/editor implementation changes are
introduced by these two upstream commits.

Both changesets configuration and the released-changeset cleanup are adopted.
The existing manual private signing/release workflow remains authoritative;
there is no switch to Grafana's public publication workflow. Workflow analysis,
SHA inventory, repository governance, endpoint handling and SDK-safe error
classification remain present.

The integration was prepared with `--no-commit` and subsequently committed as
`8de7ba8` (`chore(deps): merge upstream 2.11.0 updates`). The follow-up review on
2026-11-23 confirmed the upstream dependency graph and corrected workflow-directory
404 handling: unavailable source access no longer reports a complete empty scan.
Validation results are recorded in the dashboard-publisher review evidence for
each date. Test execution and release archives remain excluded from these reviews;
signing, publication and deployment were not performed by the integration step.

The following section is historical evidence for the earlier integration.

# Upstream alignment — 2026-11-16

The private RWE fork incorporates `grafana/github-datasource` main through
[`6f39c6429a7d9ab6a18e447ef51a2bf2cd1611f5`](https://github.com/grafana/github-datasource/commit/6f39c6429a7d9ab6a18e447ef51a2bf2cd1611f5).
The common base was `bf144c070d86116f5bc84359dcb48eca42dc7e5b`; this integration
adds 36 upstream commits. Upstream's release base is 2.10.0 and the prepared
private release is **2.10.0-rwe.1**.

## Included upstream work

- Check Runs, Pull Requests for Branches and Webhook Deliveries query types,
  including their backend models, client methods, handlers, editor controls and tests.
- Multi-value macro handling for whitespace and interpolation of string arrays,
  while retaining literal comma-containing branch names.
- Dependency/security updates through the pinned main commit, including the
  gRPC connection-freeze fix for heavy query load.
- Go 1.27.3, Node 24.22.3, npm 12.2.1, Grafana frontend libraries 13.1.9,
  plugin SDK 0.299.0 and datasource configuration schema tooling 0.0.13.

## RWE behavior retained

- Plugin/package identity `rwe-github-datasource` and private signing/release
  targets. The Grafana public publication workflow is not adopted.
- Enterprise Server and Enterprise Cloud data-residency endpoint resolution,
  including the dedicated REST/API hostname and App token exchange.
- Existing App/PAT settings, string/numeric App ID parsing, configuration schema,
  generated schema artifacts and their conformance coverage.
- Repository metadata/custom-property queries, code-search governance queries,
  property filtering and complete-result checks.
- `Workflow_Jobs` and `Workflow_Steps`, bounded run lists and exact run/attempt/job
  identity, plus their corrected result-reference and HTTP contract coverage.

Seven textual conflicts were resolved semantically: the two CI workflows,
package/lock metadata, changelog, query model and query editor. Upstream's full
query-type set and the private fork's query types coexist. The package lock
uses the upstream dependency graph with RWE package/version metadata; obsolete
private dependency overrides are not retained over the upstream security updates.

The private CI installs the npm version declared by package.json and uses
current pinned official checkout/setup actions. The Grafana-internal plugin
updater is restricted to the Grafana organization because that workflow relies
on Grafana's own GitHub App. It is not a private-fork release mechanism.

## Verification

The integration was validated in an isolated checkout with the declared Node,
npm and Go versions before being applied to the working branch:

- `go test ./pkg/...`: passed, including private endpoint/schema/governance and
  workflow-result coverage and the upstream feature tests.
- `npm run test:ci`: 10 suites, 33 tests passed.
- `npm run typecheck`, `npm run lint`, `npm run spellcheck`: passed.
- `npm run build`: passed.
- `go run github.com/magefile/mage -v build:linux` and
  `build:linuxARM64`: passed. Both binaries retain the RWE ID and release version.
- Existing uncommitted workflow-test fixes were backed up and included. Their
  source content is preserved through this integration.

`npm audit` still reports 34 upstream dependency findings (3 low, 31 moderate;
none high or critical). These are not claimed to be resolved by this merge.
No additional forced major dependency changes were applied beyond upstream.
The plugin has not been released, signed or deployed by this integration.

Use the existing private release process, then update/sync the prepared Sphere
Grafana plugin pin only after the signed immutable release is available. Existing
GitHub App permissions are not changed by merging new query types; individual
queries still need their documented read permissions.
