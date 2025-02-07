# PgBoundary - A wrapper around `boundary` and `pgbouncer` CLI for integration into IDE/database tooling

This is a simple wrapper around the `boundary` and `pgbouncer` CLI tools.

## Integration into IDE/Database tooling

This project is for a specific use case

- You have an internal postgres database
- You connect to it via HashiCorp Boundary
- You are using OIDC for AuthZ and AuthN
- You are using HashiCorp Vault integration in Boundary to provide dynamic credentials
- You want to use this setup from an IDE or other database tooling

### Idea
For each session, Boundary provides a triplet of dynamic information - `username`, `password` and localhost:`port`

Using a local [PgBouncer](https://www.pgbouncer.org/) postgres proxy, the connection settings for your tooling can be
stable while the dynamic portions of Boundary are hidden.

1. Boundary connection to an environment is established (via CLI)
2. Connection details are written into PgBouncer format
3. PgBouncer is started/reloaded
4. IDE connects through PgBouncer

![](pgboundary.png)

### Boundary Introduction

Boundary CLI and Desktop clients are available here: https://developer.hashicorp.com/boundary/install

Boundary has the concept of `Scopes` and `Targets`.

#### Scope

Scopes are used to define areas inside Boundary and are used for **Authentication** and for selecting **Targets**.

These 2 scopes will most likely NOT be the same and can be set in the `pgboundary.ini` either as defaults (`[scopes]`) or per `[target]`.

#### Target

A target is a configuration item inside Boundary defining to which entity a connection should be established and what credentials to provide.

### Setup

1. Install dependencies CLI

    - PgBouncer via [brew](https://formulae.brew.sh/formula/pgbouncer)
      ```
      brew install pgbouncer
      ```
    - [Boundary CLI](https://developer.hashicorp.com/boundary/install) via brew
      ```
      brew tap hashicorp/tap
      brew install hashicorp/tap/boundary
      ```

2. Download the latest release from the [releases page](https://github.com/sigterm-de/pgboundary/releases)

3. Copy the files `pgboundary.ini`, `pg_config.ini` and `pg_auth` to a convenient place. The binary tries to find them in the following locations:
    - `./`
    - `~/.pgboundary/`, or
    - `$XDG_CONFIG_HOME/pgboundary`

   Note: you can always specify a config file with `-c path/to/config.ini`

4. Configuration

   The configuration file (`pgboundary.ini`) consists of three main sections:

    ```ini
    [pgbouncer]
    workdir = /usr/local/var/pgbouncer
    conffile = pg_config.ini

    [scopes]
    auth = global    # Default authentication scope
    target = global  # Default target scope

    [targets]
    # Basic target with global scopes
    demo-dev = host=https://boundary.example.com target=demo-ro

    # Target with custom scopes and database
    demo-stage = host=https://boundary.stage.example.com target=demo-ro auth=org scope=dev database=testdb
    ```

   Each target entry consists of:
    - `host`: Boundary server URL (including https://)
    - `target`: Boundary target name
    - `auth`: (optional) Authentication scope, overrides default
    - `scope`: (optional) Target scope, overrides default
    - `database`: (optional) Database name, defaults to target name without "-ro" suffix

5. Configure your IDE/database tool:
    - Host: `127.0.0.1`
    - Port: `5432` (as configured in `pg_config.ini`)
    - Username/password: as set in `pg_auth`
    - Database: use the target name from `pgboundary.ini`

## Usage

```bash
# List available targets and active connections
pgboundary list

# Connect to a target
pgboundary connect demo-dev

# Show verbose output
pgboundary -v connect demo-dev

# Shutdown specific connection
pgboundary shutdown demo-dev

# Shutdown all connections
pgboundary shutdown
```

### Configuration Tips

- For shared database instances, specify the database name in the target configuration
- Scopes can be set globally in the `[scopes]` section or per-target
- Use the verbose flag (`-v`) for debugging connection issues

## Limitations

- Only OIDC authentication is supported

## License

MIT

## TODO

- [x] `shutdown` // remove external binaries (`pgrep`, `pkill`)
- [x] `connect` // remove external binaries for authentication  
  I'll leave the call to boundary CLI in as handling the possibly different AuthN methods would be re-inventing the wheel.  
  See: https://github.com/hashicorp/boundary/tree/main/internal/cmd/commands/authenticate
- [ ] `connect` // support individual auth methods per target
- [ ] more tests