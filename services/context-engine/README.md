# Context Engine (optional, opt-in)

Semantic + graph code search for SuperCoder. The desktop app stays zero-backend
by default; enabling the **Context engine** toggle in Settings means "run this
compose stack." The app then streams your repo here for indexing and the agent's
`codebase_search` / `codebase_graph` tools query it.

## Run the stack

```bash
cd services/context-engine
cp .env.example .env          # set SUPERCODER_OPENAI_API_KEY (server-side embedding key)
docker compose up -d --build
```

Brings up: `postgres`, `redis`, `qdrant`, `falkordb`, a one-shot `migrate`
(Atlas), and the `context-engine-server` (HTTP on **:8106**) + `context-engine-worker`.

Check it's healthy:

```bash
docker compose ps
curl -fsS http://localhost:8106/api/health
docker compose logs context-engine-server | grep -i "streaming sync endpoints registered"
```

Then in the desktop app: **Settings → Context engine → enable**, leave the port
at `8106`, and open a coding session on a repo. The app streams it up; once
indexing reaches `done`, `codebase_search` returns results.

## Notes

- **Embedding key** lives here (`.env` → `SUPERCODER_OPENAI_API_KEY`), never in the app.
- **Merkle trees** persist to the shared `merkle` Docker volume (local-disk CAS,
  flock + sha256). No S3/MinIO. The server and worker share this volume so
  incremental `/index/diff` sees what the worker's finalizer committed.
- **Port** is editable in Settings (default `8106`); the app builds
  `base_url = http://127.0.0.1:<port>`.
- Tear down with `docker compose down` (add `-v` to drop the indexed data volumes).
