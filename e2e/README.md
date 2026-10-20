# Browser smoke test

Playwright covers the path that cannot be unit-tested: a 390×844 viewport, a
reload mid-feed, OOB swaps, delete/undo in place, and the export endpoints.

```sh
cd e2e
npm install
npx playwright install chromium
npm test
```

The config starts its own server on `127.0.0.1:8097` with
`NURTUR_LOCAL_ONLY=true`, writing `.e2e/nurtur.db` (gitignored). Delete that
file for a clean run.

Not automated here, and still part of the release gate:

- 24-hour soak with repeated mutations, watching goroutine/FD counts and WAL size
  (`/healthz?verbose=1`);
- Turso outage drill: disable the network, keep logging, restore the network, then
  compare cloud row counts with local;
- backup restore on a clean machine (see the root README);
- iOS Safari and Android Chrome home-screen install and safe areas;
- the dark-room "3 a.m." check.
