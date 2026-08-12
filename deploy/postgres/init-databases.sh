#!/bin/bash
# Bootstraps one database and one role per service inside a single Postgres
# instance.
#
# One container per service does not survive six services on a laptop. A shared
# instance with a database and a login role per service keeps the isolation
# contract -- no service can read another's tables -- while cutting the
# container count to one.
#
# This runs only on first initialisation of the data directory, which is
# correct for role and database bootstrap. Application schema is owned by
# golang-migrate and applied separately on every start, so a schema change
# never requires wiping the volume.
#
# Identifiers and passwords reach SQL through psql's :'var' interpolation and
# format()'s %I/%L, never through shell substitution into the statement text.
# This runs as the superuser at first initialisation, so a password containing
# a quote must not be able to alter the statement it appears in.
set -euo pipefail

create_service_database() {
    local db="$1"
    local role="$2"
    local password="$3"

    echo "Creating database ${db} owned by ${role}"

    # \gexec runs each generated statement. format() quotes the identifier with
    # %I and the password with %L, so neither can break out of its position.
    #
    # None of these SELECTs is semicolon-terminated, and that is deliberate:
    # \gexec runs the *current query buffer*, so a trailing ';' would send and
    # clear the buffer first, leaving \gexec nothing to execute. The generated
    # statement would then simply be printed and the role never created --
    # which fails later, confusingly, as "database does not exist".
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
        -v role="$role" -v password="$password" <<-'EOSQL'
        SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'role', :'password')
         WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'role')
        \gexec
EOSQL

    # CREATE DATABASE cannot run inside a transaction block, so it is issued
    # separately and guarded by a lookup rather than by an exception handler.
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
        -v db="$db" -v role="$role" <<-'EOSQL'
        SELECT format('CREATE DATABASE %I OWNER %I', :'db', :'role')
         WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'db')
        \gexec
EOSQL

    # Fail loudly here rather than letting a silently-skipped \gexec surface as
    # a connection error three steps later.
    if ! psql --username "$POSTGRES_USER" --dbname postgres -tAc \
        "SELECT 1 FROM pg_database WHERE datname = '${db}'" | grep -q 1; then
        echo "FATAL: database ${db} was not created" >&2
        exit 1
    fi

    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "${db}" \
        -v db="$db" -v role="$role" <<-'EOSQL'
        SELECT format('GRANT ALL PRIVILEGES ON DATABASE %I TO %I', :'db', :'role')
        \gexec
        SELECT format('GRANT ALL ON SCHEMA public TO %I', :'role')
        \gexec
EOSQL
}

create_service_database "identity_db" "${IDENTITY_DB_USER:-identity_user}" "${IDENTITY_DB_PASSWORD:-password}"
create_service_database "football_db" "${FOOTBALL_DB_USER:-football_user}" "${FOOTBALL_DB_PASSWORD:-password}"

echo "Service databases ready"
