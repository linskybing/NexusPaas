# Twelve-Factor Rule

Use these rules when adding runtime behavior, deployment code, workers, cloud adapters, model
serving, or operational tooling for the shared-fridge mistake-prevention system.

The baseline source is [The Twelve-Factor App](https://12factor.net/). This document narrows those
principles to this repository's Python, edge, AWS, privacy, and deterministic-test constraints.

## Project Rule

Runtime code must be portable, explicitly configured, stateless unless state is held in an attached
resource, and safe to run in local tests without cameras, model weights, AWS credentials, or network
access.

## Factors

1. **Codebase** Keep required behavior in versioned project files. Do not rely on untracked
   notebooks, local scripts, shell history, manually exported schemas, or machine-local model state
   for the app to run.

2. **Dependencies** Declare Python dependencies in project metadata or lock files. Do not depend on
   globally installed packages, developer-specific virtual environments, or implicit AWS/ML SDK
   availability.

3. **Config** Store deploy-specific values in environment variables or typed settings. Do not
   hard-code fridge IDs, AWS account IDs, S3 buckets, MQTT endpoints, thresholds, secrets, model
   paths, or retention periods in domain logic.

4. **Backing services** Treat AWS services, cameras, object stores, databases, model servers, and
   message buses as attached resources behind adapters. Domain modules should accept contracts and
   ports, not concrete clients.

5. **Build, release, run** Keep build steps, release configuration, and runtime execution separate.
   Do not download model weights, mutate schemas, create buckets, or run migrations at import time.

6. **Processes** Keep API, worker, inference, edge capture, and cleanup processes stateless. Persist
   inventory, idempotency, media references, audit records, and review state in explicit stores.

7. **Port binding** Services must expose explicit ports or handler entrypoints. Avoid hidden
   background daemons or import-time listeners. Containerized HTTP services should preserve the
   documented health and inference routes.

8. **Concurrency** Scale by process type. Edge capture, inference, API, worker, and retention
   cleanup should remain independently runnable and horizontally replaceable where practical.

9. **Disposability** Start fast, stop safely, and handle interrupted processing through idempotent
   retries. Workers must tolerate duplicate S3/MQTT events and must not leave half-applied ownership
   or privacy state.

10. **Dev/prod parity** Keep local, test, staging, and production flows aligned through the same
    contracts and adapters. Tests should use deterministic fakes rather than weakening production
    interfaces.

11. **Logs** Write operational facts to stdout/stderr or structured log sinks. Do not hide important
    state in local files, ad hoc reports, or side-channel printouts that production cannot collect.

12. **Admin processes** Run schema export, backfills, retention cleanup, benchmark generation, and
    one-off repair tasks as explicit admin commands. They must share project settings and contracts
    with the app.

## Review Checklist

- New runtime config is represented in typed settings or environment handling.
- External systems remain behind adapters and are replaceable in tests.
- Startup has no network calls, schema mutations, bucket creation, or model downloads unless it is
  an explicit run step.
- Workers are idempotent and can be killed between events without corrupting inventory, ownership,
  privacy, or audit state.
- Tests remain deterministic and do not require real AWS credentials, cameras, model weights, or
  network access.
