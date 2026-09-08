# G0 Runtime Fixture

`docker-compose.yml` starts the deterministic PostgreSQL and mapping-boundary
fixtures with a candidate source tree. The evaluator supplies `CANDIDATE_ROOT`
and a unique `APP_PORT`; neither the source answer nor any evaluator is mounted
inside the application container.
