// Railway infrastructure for the GymPulse API. Apply it once per environment
// with an environment-scoped token: `railway config plan`, then
// `railway config apply`. See docs/RELEASE_RUNBOOK.md.
import { defineRailway, image, preserve, project, service } from "railway/iac";

export default defineRailway((ctx) => {
  const environment = ctx.environmentName ?? ctx.environment;
  if (environment !== "staging" && environment !== "production") {
    throw new Error(`gym-pulse-api deploys only to staging or production, not ${environment}`);
  }

  const api = service("api", {
    // The deploy workflow moves this tag to the accepted image digest and then
    // redeploys, so staging and production run the same build. Auto-updates
    // stay off: only the workflow decides what ships.
    source: image(`ghcr.io/yadieloscar/gym-pulse-api:${environment}`, {
      autoUpdates: { type: "disabled" },
    }),
    // Pending migrations run before the new version takes traffic. A failure
    // stops the deployment and leaves the previous version serving.
    preDeploy: "./migrate",
    healthcheck: "/ready",
    healthcheckTimeout: 120,
    deploy: {
      restartPolicyType: "ON_FAILURE",
      restartPolicyMaxRetries: 5,
      // Longer than the server's 10s graceful shutdown, so in-flight requests
      // finish before the old container is killed.
      drainingSeconds: 15,
    },
    env: {
      ENVIRONMENT: environment,
      LOG_LEVEL: "info",
      // Secrets and per-environment values are set in Railway, never here.
      DATABASE_URL: preserve(),
      DATABASE_LOCK_URL: preserve(),
      SUPABASE_URL: preserve(),
      SUPABASE_SERVICE_ROLE_KEY: preserve(),
      SUPABASE_JWT_AUDIENCE: preserve(),
      ALLOWED_ORIGINS: preserve(),
    },
  });

  return project("gym-pulse", { resources: [api] });
});
