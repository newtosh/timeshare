# Security model

- **Peer verification.** The daemon's Unix socket is `0600` and verifies
  the connecting process's UID before serving any request (`SO_PEERCRED` on
  Linux, `LOCAL_PEERCRED`/`Xucred` on macOS). A second local user can't
  connect at all, not merely fail to authenticate.
- **Item allow-list is a blast-radius control, not an adversarial
  boundary.** The daemon trusts the client-supplied allow-list from
  `.timeshare.yml` (the file the CLI reads locally and forwards). This is
  intentional and sound *because* of the peer-UID check above: a same-UID
  process on your own machine could edit `.timeshare.yml` directly anyway,
  so the allow-list's job is to prevent an accidental over-broad read (a
  script that only expects `DATABASE_URL` can't also pull `STRIPE_KEY`
  through the same daemon), not to defend against a local attacker who
  already has your UID.
- **No secret ever touches disk.** The cache is in-memory only; a daemon
  restart cold-starts it. `.timeshare.yml` never contains a credential. A
  service-account token lives in the OS keychain or an env var, never a
  file `timeshare` writes.
- **TTL is wall-clock, not a timer.** Cache expiry survives laptop
  sleep/resume correctly by construction — it's a comparison against the
  clock at read time, not a callback that could be missed while asleep.

## Reporting a vulnerability

This is an early-stage, pre-1.0 project. If you find a security issue,
open an issue on GitHub describing the problem — there's no dedicated
security contact or disclosure process yet given the project's current
size and stage.
