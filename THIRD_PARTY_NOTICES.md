# Third-party notices

NiriDesk does not vendor third-party source code. Dependencies are resolved by
Go modules at compile time and by pnpm from `web/pnpm-lock.yaml` for the browser
application. Generated directories such as `web/node_modules/` and `web/dist/`
are not tracked.

## Browser application

- **noVNC 1.7.0**, Mozilla Public License 2.0, <https://github.com/novnc/noVNC>
- **pako** (bundled by noVNC), MIT License, <https://github.com/nodeca/pako>
- **SolidJS 1.9.17**, MIT License, <https://github.com/solidjs/solid>
- **Vite 8.3.5**, MIT License, <https://github.com/vitejs/vite>
- **vite-plugin-solid 2.11.14**, MIT License, <https://github.com/solidjs/vite-plugin-solid>
- **TypeScript 7.0.2**, Apache License 2.0, <https://github.com/microsoft/TypeScript>

NoVNC, its bundled licenses, and the pako license are copied into
`web/dist/licenses/` by `web/scripts/copy-licenses.mjs` during a production
build, so the deployed application ships the applicable notices.

## Go server

- **gorilla/websocket v1.5.3**, BSD 3-Clause License, <https://github.com/gorilla/websocket>

Exact versions and integrity hashes for all JavaScript dependencies are pinned
in `web/pnpm-lock.yaml`. Go dependency versions are pinned in `go.mod` and
`go.sum`.
