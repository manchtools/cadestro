# Contributing to Cadestro

Run the complete sequential gate:

```bash
./scripts/verify-all.sh
```

Generated protobuf and sqlc output must be regenerated from its source and
never edited directly. Every Go module must build with `GOWORK=off`.
