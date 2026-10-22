-- Local/CI only. Mounted into the PostgreSQL container's initdb directory so
-- the gym_pulse_api login exists before the API runs migration 026, mirroring
-- the production rollout order (create role, grant, deploy, verify). The
-- credential is a throwaway for disposable databases; production sets its own.
CREATE ROLE gym_pulse_api LOGIN PASSWORD 'gym_pulse_api'
    NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
