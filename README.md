# CPHDRIVE

A private, self-hosted drive. This is a fork of
[File Browser](https://github.com/filebrowser/filebrowser) v2.63.23, rebuilt for one
server: a Drive-style rail, a New dialog, permanent share links, and HTML previews.

Everything still runs from a single static Go binary. Files are served straight
from disk, with no database of contents and no cloud account.

## What is different from upstream

The authoritative list lives at the top of `build.sh`. In short:

- Drive-style sidebar with one **New** pill that opens a centered dialog
  (New folder, New file, Upload file, Upload folder).
- Apple-style pass in `frontend/src/css/apple.css`: fluid type and rail width,
  press states, materials, reduced-motion fallbacks.
- Share links can be created with no expiry ("Never"), and copied share links
  are rewritten to the public host.
- HTML files open in the editor with a preview route that renders them in a
  sandboxed iframe. The raw response carries a CSP `sandbox` directive so the
  document keeps an opaque origin even when opened directly.
- The interface is branded CPHDRIVE end to end. No upstream name, artwork, or
  outbound links remain.

## Build

Requires Go 1.26 and Node 24.

```sh
./build.sh
```

That produces `filebrowser-linux-386` for the target server. To build for the
machine you are on, run the two steps yourself:

```sh
cd frontend && pnpm install && pnpm build && cd ..
go build -o filebrowser .
```

## The design is runtime, not compiled

`branding/custom.css` and `branding/img/` are served at runtime from the
directory named by the database setting `branding.files`. A plain source
rebuild will look unstyled until that setting points at a copy of `branding/`:

```sh
filebrowser -d filebrowser.db config set \
  --branding.files /path/to/branding \
  --branding.name CPHDRIVE \
  --branding.theme dark
```

Any file under `branding/img/` overrides the embedded asset of the same path,
and only that path. Missing files fall back to the binary.

## Deploy

```sh
scp filebrowser-linux-386 host:~/fb-new
ssh host 'sudo install -m 755 ~/fb-new /usr/local/bin/filebrowser && sudo systemctl restart filebrowser'
```

Copy `branding/` alongside the binary the first time, and again whenever the
stylesheet or the icons change.

## Security

Upstream is archived and states that some issue classes will not be fixed:
command execution through the runner and hooks, and session/JWT handling.
Neither has been addressed here.

- Leave the command runner disabled (it is off by default).
- Do not expose the instance to the internet without authentication in front
  of it. Publishing only the share paths is the narrowest option.
- Run it as an unprivileged user with only the served directory readable.

## Licence

Apache 2.0, inherited from the upstream project. See `LICENSE`.
