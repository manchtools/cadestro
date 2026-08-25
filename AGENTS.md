# Project audit rules

## Class-wide coverage

For every repository-wide, module-wide, or polymorphic API audit, enumerate all
modules, backends, and implementations before drawing conclusions. Apply the
same parser-backed structural query and necessity-versus-complexity review to
every item, record an explicit verdict for every coverage cell, and report any
cell that could not be checked. User-supplied examples seed the search class;
they never define or limit its scope. Before recommending that a concept be
preserved or reintroduced, check repository history and current operator
rulings for that concept.

Every reported zero in an audit must be corroborated by a second independent
method, and the report must name both methods. A silent zero from one parser is
not evidence that a class is absent.

## Agent and process isolation

Give every concurrent agent its own scratch namespace and require it to verify
that re-read artifacts belong to its assigned scope. Never terminate an
externally owned agent from a broad process listing: resolve the exact PID or
session ID explicitly authorized by the operator, and leave every unmatched
process running regardless of model name or age.

## Recorded operator rulings

Ordinary authored actions are assigned and pulled during sync. Do not preserve
or reintroduce server-push dispatch for actions, action sets, definitions, or
groups, including durable one-shot delivery built only for that path. Push is
for genuinely live operations such as OSQuery, reboot, and terminal traffic.

Goose migrations are the product's schema mechanism, and sqlc consumes that
canonical migration history for generated queries. The embedded Goose runner is
the automatic upgrade mechanism; never describe Cadestro as lacking migration
machinery. Before 1.0, unreleased transient history may be squashed into a reset
point, but an already released schema is immutable: every later schema change is
a new ordered migration with tested upgrade and rollback behavior. Do not
replace product migrations with a runtime baseline schema file.
