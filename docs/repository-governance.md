# Repository governance queries

Version `2.11.0-rwe.1` adds `Repository_Governance` to the existing App/PAT connection. It reads one organization snapshot on demand and rebuilds frames for each caller. No new credentials, collector, persistent history or recording rules are introduced.

Inputs are `owner`, optional exact `repository` for details, and `options.view` (`ownership`, `repositories`, `details`). `options.ownership` filters repository rows; `*` or empty means all, and `__unassigned__` groups missing, empty and Unknown ownership. Ownership summaries always cover the whole retrieved organization population. The detail guard rejects a repository outside the retrieved scope or selected ownership.

`ownership` emits repository counts, repositories with review findings and unchecked-configuration counts, plus a stable ownership key for links. Declared ownership values remain visible even when their retrieved count is zero. `repositories` emits one row per retrieved repository with responsibility, review reason and configuration-search evidence. No background scan is needed to follow ownership links: views share the bounded 30-second instance-local cache.

A review finding means unresolved ownership or a complete search without an exact `.github/settings.yaml` match. The latter calls for source verification; it does not prove the file is absent. Search errors and incomplete retrieval produce warnings and explicit unknown configuration states. They are not counted as successful checks. Positive indexed matches remain usable during incomplete search. Repository-list errors return a query error; legitimate empty listings remain empty.

Repository listing is bounded to 20 pages of 100 repositories. Code search is bounded to 10 pages of 100 results, with GitHub completeness flags and observed totals retained. The search expression contains the explicit organization, without nested textbox variables. Schema access errors and rate limits remain visible. A schema rate limit prevents additional configuration search work.

`details` reads only the selected repository's settings and reachable inheritance through the configured API host. Sources resolve to commits and source links retain those commits. Limits are 16 API requests, four inheritance levels beyond the root and 1 MiB per file. Paths and repository references are validated, cycles stop, and access or source failures remain explicit. A file not returned by the API is not treated as proof of absent access or absent configuration.

Self-contained settings are valid. Inheritance applies parent custom properties before child overrides. Reported repository custom properties are compared to declarations only when inheritance resolves completely; a matching property is not a compliance score. Rulesets, permissions, secrets, workflow outcomes and reconciliation status are not assessed. Selected-repository inspection does not fan out across the portfolio.

The Governance dashboard keeps three views: ownership counts, one full-width repository table and selected configuration details opened from a contextual table link. The source uses the selected GitHub datasource consistently and defaults to organization `rwest`. Supporting private/archive cards, custom-property schema and technical search inputs were removed as requested.

Property comparison treats multi-select values as an unordered list and preserves scalar/list distinctions. YAML booleans and their GitHub string representation compare consistently. Unsupported declaration or observed value types remain explicitly non-comparable rather than producing a misleading match.
