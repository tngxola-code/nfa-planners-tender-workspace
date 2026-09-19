-- Runs once when the Postgres data volume is empty.
-- Creates the two databases the local stack needs.

-- nfa is created by the POSTGRES_DB env var, so it already exists.
-- keycloak is Keycloak's own database, separate from the application data.

CREATE DATABASE keycloak OWNER nfa;
