# CLI Reference

```bash
go install github.com/grokify/omniroadmap/cmd/omniroadmap@latest
```

All commands share the store flags:

| Flag | Default | Purpose |
|---|---|---|
| `--dsn` | resolved (see below) | MySQL-wire DSN for the dolt sql-server |
| `--port` | resolved (see below) | dolt sql-server port; ignored when `--dsn` is set |
| `--data-dir` | resolved (see below) | Dolt data directory (used when starting a server) |
| `--start-server` | `true` | Start a local `dolt sql-server` if none is reachable |

DSN/port/data-dir resolve in order: CLI flag > environment variable
(`OMNIROADMAP_DSN`, `OMNIROADMAP_PORT`, `OMNIROADMAP_DATA_DIR`) > config
file (`~/.omniroadmap/config.json`) > built-in default (port `13307`,
data dir `~/.omniroadmap`). See [`omniroadmap config`](#omniroadmap-config)
below — useful when running several Dolt-backed tools side by side (e.g.
[visionstudio](https://github.com/ProductBuildersHQ/visionstudio) on port
13306) and you want to guarantee no port collision.

## `omniroadmap sync`

Sync one provider into the canonical store.

```bash
omniroadmap sync --provider <name> [flags]
```

| Flag | Purpose |
|---|---|
| `--provider` | Required: `aha`, `aha-studio`, `productboard`, or `jpd` |
| `--fieldmap` | Per-tenant fieldmap JSON (custom fields → MoSCoW/RICE) |
| `--kinds` | Comma-separated item kinds (default: all the provider supports) |
| `--cache-db` | aha-studio provider: cache DB path (default `~/.ahastudio/cache.db`) |
| `--product` | aha-studio provider: scope to one Aha product/workspace |

Provider credentials come from environment variables:

| Provider | Environment |
|---|---|
| `aha` | `AHA_SUBDOMAIN`, `AHA_API_KEY` |
| `aha-studio` | none (reads the local cache file) |
| `productboard` | `PRODUCTBOARD_API_TOKEN` |
| `jpd` | `JIRA_URL`, `JIRA_USER`, `JIRA_TOKEN` |

Examples:

```bash
# From the local aha-studio cache, with tenant prioritization mapping
omniroadmap sync --provider aha-studio --fieldmap tenant-acme.fieldmap.json

# Only features from the live Aha API
omniroadmap sync --provider aha --kinds feature

# From a copy of the cache (e.g. for testing)
omniroadmap sync --provider aha-studio --cache-db /tmp/cache-copy.db
```

## `omniroadmap db init`

Create the omniroadmap database and schema (starts a local dolt
sql-server if needed). Syncs run this implicitly; use it to prepare a
store ahead of time.

```bash
omniroadmap db init
```

## `omniroadmap status`

Report last-sync time and record counts per provider and kind (from the
`sync_meta` table).

```bash
$ omniroadmap status
aha-studio     epic            152 records  last sync 2026-08-13 07:15:10
aha-studio     feature        6980 records  last sync 2026-08-13 07:15:08
aha-studio     initiative      626 records  last sync 2026-08-13 07:15:15
aha-studio     release           0 records  last sync 2026-08-13 07:15:15
```

## `omniroadmap augment`

Manage locally-authored data (MoSCoW, Kano, RICE, OKR refs, notes) keyed
by provider and source reference — e.g. an Aha reference like
`MYPROJ-123`. Augments are stored apart from synced provider data and
survive every re-sync; see [Augments](augment.md).

```bash
# Set fields (merges with the existing augment; "" clears a field):
omniroadmap augment set --provider aha-studio --ref MYPROJ-123 \
  --moscow must_have --kano performance \
  --reach 5000 --impact 2 --confidence 0.8 --effort 3 \
  --okr OKR-2026-Q3-01 --notes "exec ask"

omniroadmap augment get  --provider aha-studio --ref MYPROJ-123
omniroadmap augment list [--provider aha-studio]
omniroadmap augment rm   --provider aha-studio --ref MYPROJ-123
```

Each `set`/`rm` ends with a Dolt commit, so augment history is versioned.

## `omniroadmap item get`

Read one canonical item — synced provider data with the augment overlay
applied — by source reference, source ID, or canonical ID. `--provider`
is only needed when a ref is ambiguous across providers.

```bash
omniroadmap item get MYPROJ-123
omniroadmap item get --provider aha-studio MYPROJ-123
```

## `omniroadmap config`

Show the resolved store configuration — DSN, port, and data directory —
and where each value came from (flag/env/config/default). Useful for
confirming which port a command will actually use before it starts a
server.

```bash
$ omniroadmap config
Config file: /Users/johnwang/.omniroadmap/config.json (loaded)

  dsn       root:@tcp(127.0.0.1:13307)/omniroadmap        (from config)
  port      13307                                         (from config)
  data-dir  /Users/johnwang/.omniroadmap                  (from default)
```

`omniroadmap config set-port <port>` persists a port to
`~/.omniroadmap/config.json`, so every subsequent command (and any
auto-started `dolt sql-server`) uses it without needing `--port` or
`--dsn` on every invocation:

```bash
omniroadmap config set-port 13309
```

This matters when running more than one Dolt-backed tool locally — e.g.
[visionstudio](https://github.com/ProductBuildersHQ/visionstudio) already
defaults to port 13306, and omniroadmap defaults to 13307. If you add a
third Dolt-backed tool, or want omniroadmap on a non-default port for any
other reason, set it once here rather than passing `--port`/`--dsn` on
every command. You can also hand-edit the file directly:

```json
{
  "port": 13309
}
```

Full config schema (all fields optional):

| Field | Purpose |
|---|---|
| `port` | dolt sql-server port |
| `database` | Dolt database name (default `omniroadmap`) |
| `data_dir` | Dolt data directory (default `~/.omniroadmap`) |
| `dsn` | Full DSN, overriding `port`/`database` entirely |

## Inspecting the store directly

The store is a normal Dolt database — the `dolt` CLI and any MySQL client
work against it:

```bash
cd ~/.omniroadmap/omniroadmap
dolt log            # every sync run is a Dolt commit
dolt sql -q "SELECT kind, COUNT(*) FROM items GROUP BY kind"
```
