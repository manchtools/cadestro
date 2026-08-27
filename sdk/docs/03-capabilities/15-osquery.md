---
title: osquery
label: osquery
description: Query host state with osquery's SQL interface — with a deny-list that keeps a compromised caller out of credential-bearing tables.
icon: "🔎"
---

# osquery

`sys/osquery` exposes the host as SQL through `osqueryi`: list tables, query a
table, or run raw SQL. It is a single-tool capability, so `New` takes only a
Runner, no Backend. A **deny-list** refuses credential-bearing tables on the
convenience path, which is the part to pay attention to.

## Construct a querier

```go
r, err := exec.NewRunner(exec.Sudo) // osquery reads privileged tables
if err != nil {
    return err
}
q, err := osquery.New(r) // ErrNotInstalled if osqueryi is absent
if err != nil {
    return err
}
```

## Query

```go
rows, err := q.QueryTable(ctx, "os_version") // []osquery.Row, one map per row
tables, err := q.ListTables(ctx)
rows, err = q.QuerySQL(ctx, "SELECT name, pid FROM processes")
```

A `Row` is `map[string]string` — exactly the element shape of osqueryi's
`--json` output. Refusals are `errors.Is`-able sentinels:
`ErrTableNotPermitted` (deny-list), `ErrInvalidTableName` (identifier shape,
including the 64-byte name cap), and `ErrQueryTooLong` (raw SQL over 4096
bytes). The wrapped error always names the offending table.

<!-- docref: begin src=sys/osquery/osquery.go#client.QueryTable:ac73160b -->
`QueryTable` validates the table name (alphanumeric + underscore, capped
length) and applies the deny-list before building `SELECT * FROM <table>`, so
an invalid or forbidden name is rejected without running anything.
<!-- docref: end -->

## The sensitive-table deny-list

<!-- docref: begin src=sys/osquery/osquery.go#sensitiveTables:1054224b -->
A curated deny-list refuses tables that expose credential material — `shadow`
(password hashes), `process_envs` (secrets in process environments), `crontab`,
`shell_history`, and `sudoers`. They all pass the name validity check, so a
shape-only filter is not enough; this refuses them by name so hostile query
input cannot exfiltrate them through privileged osquery.
<!-- docref: end -->

<!-- docref: begin src=sys/osquery/osquery.go#client.QuerySQL:17f00532 -->
The deny-list gates **every** query path: `QueryTable` refuses a sensitive
name before building any SQL, and `QuerySQL` refuses raw SQL that *references*
a sensitive table as a whole-word identifier anywhere in the query — the scan
fails closed rather than parsing SQL. There is no osquery path to a credential
table.
<!-- docref: end -->

{% callout type="warning" title="Raw SQL is still the caller's responsibility" %}
`QuerySQL` runs arbitrary read-only SQL, so restrict which of your callers can
reach it. It is gated by the credential deny-list — it cannot read
`shadow`/`sudoers`/… — but it can still read any other table osquery exposes.
{% /callout %}

## Related

- [Antivirus](/capabilities/antivirus) — malware scanning alongside host queries.
- Inventory (`sys/inventory`) — structured hardware/software facts without SQL.
