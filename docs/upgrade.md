# Upgrades

Set `IMAGE_TAG` in `server/deploy/.env` and run
`server/deploy/deploy.sh`.

The deployment re-renders configuration, validates Compose, pulls the images,
and waits for healthy services. Control applies pending ordered Goose
migrations before serving traffic.

Cadestro is pre-1.0. Unreleased migration history may be squashed, but a
released migration is immutable; later schema changes require a new ordered
migration with upgrade and rollback tests.
