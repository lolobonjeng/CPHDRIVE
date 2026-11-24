#!/usr/bin/env bash
# Rebuild CPHDRIVE File Browser from source.
#
# Base:  upstream filebrowser/filebrowser tag v2.63.23, commit e8a388f8
# Target: linux/386 (the server is Debian 12 i686)
# Output: ./filebrowser-linux-386
#
# LOCAL PATCHES vs upstream (this is the full list):
#
#   Share links
#   1. frontend/src/components/prompts/Share.vue
#      The first option in the share duration dropdown, "seconds", becomes:
#          <option value="never">Never</option>
#      The label is a literal "Never", not a translation key, matching the
#      shipped binary.
#   2. frontend/src/api/share.ts
#      Added at the top of create():
#          if (unit === "never") expires = "";
#      Clears the expiry value when the share unit is "never", so no
#      ?expires= param is sent. The backend then stores expire=0 (permanent).
#
#   HTML preview
#   3. frontend/src/views/Files.vue
#      .html and .htm open in the Editor. The rendered view lives at
#      ?preview=true and is reached from the editor's eye button.
#   4. frontend/src/views/files/Preview.vue
#      isHtml computed plus a sandboxed iframe. The sandbox attribute omits
#      allow-same-origin on purpose, so embedded scripts get an opaque origin
#      and cannot read the app's localStorage or cookies.
#      Also: editAsText() drops the ?edit= flag for HTML (the editor is the
#      default now) and updatePreview() reads route.path instead of
#      route.fullPath, because the query string was leaking into the header
#      title and breaking the listing lookup used for prev/next.
#   5. http/raw.go
#      HTML responses get "sandbox allow-scripts ..." in the CSP so scripts
#      run, while the CSP sandbox keeps the document in an opaque origin even
#      when the raw URL is opened directly. Non-HTML keeps script-src 'none'.
#   6. frontend/src/css/styles.css
#      .html-preview sizing and top padding so content clears the header bar.
#
#   Theme switcher
#   7. frontend/src/utils/theme.ts
#      getTheme() reads a localStorage override; toggleTheme() writes it, so
#      the choice survives a reload. setTheme() still does not write.
#   8. frontend/src/components/Sidebar.vue
#      Sun/moon toggle button and an About button.
#
#   About modal
#   9. frontend/src/components/prompts/About.vue   (new file)
#  10. frontend/src/components/prompts/Prompts.vue  registers "about"
#
#   Removed upstream notices
#  11. cmd/root.go          removed the startup "being wound down" log lines
#  12. frontend/src/views/Settings.vue  removed the sunset card
#
#   Apple-style pass
#  13. frontend/src/css/apple.css          (new file)
#      Imported last from styles.css so it overrides the upstream sheets
#      without editing them. Tokens for motion, radii, elevation and
#      materials; press states; reduced-motion and reduced-transparency
#      fallbacks. Delete the @import line in styles.css to revert the look.
#  14. frontend/src/components/prompts/Prompts.vue
#      Wraps BaseModal in <Transition name="prompt"> so dialogs animate in and
#      out. The leave animation needs the wrapper; v-if alone unmounts at once.
#  15. frontend/src/components/Sidebar.vue
#      Disk usage block width changed from 90% to calc(100% - 2.8em) so it
#      lines up with the inset nav rows instead of overflowing the sidebar.
#
#   Drive-style sidebar
#  16. frontend/src/components/Sidebar.vue
#      Rebuilt to the Google Drive rail: CPHDRIVE wordmark, one New pill, two
#      nav groups, disk meter, footer. New folder / New file moved into the New
#      dialog. Active row is tinted. The New pill only shows on files routes,
#      because the hidden upload inputs it drives live in the listing.
#      openNew() pops the sidebar's own prompt first, so on phones closing the
#      menu does not slide the drawer back in.
#  17. frontend/src/components/prompts/New.vue     (new file)
#      Centered dialog with four rows: New folder, New file, Upload file,
#      Upload folder. The first two swap the prompt (closeHovers + showHover)
#      instead of stacking, so Cancel lands nowhere rather than back in the
#      menu. The upload rows click #upload-input / #upload-folder-input, which
#      already existed but had no visible trigger anywhere in the UI.
#  18. frontend/src/components/prompts/Prompts.vue registers "new"
#  19. frontend/src/css/apple.css
#      New-rail styles, flat list rows (upstream paints every row with
#      --surfacePrimary, which makes a list look like stacked cards), a faint
#      fixed blue bloom behind the app in dark mode, and a fluid layout:
#        - html font-size clamp(15px..17px) so every em in the app scales with
#          the viewport instead of being pinned to 16px
#        - rail width clamp(10em..16em) instead of upstream's hard
#          16em -> 10em snap at 1024px; main is sized from the same token
#        - header search clamp(14em..34em) instead of a flat 25em
#
#   Recovered from the deployed build
#  20. frontend/public/index.html
#      Two inline scripts that the deployed binary carried and a clean
#      upstream rebuild does not have. Losing them is why a source rebuild
#      looked worse than the running app:
#        - password reveal: injects a .cf-eye button beside every password
#          input and repositions it on resize/scroll
#        - clipboard rewrite: rewrites copied /share and /api/public links to
#          the public Funnel host so a pasted link works outside the tailnet
#
#   Branding: CPHDRIVE, no upstream name
#  21. frontend/src/utils/constants.ts   name fallback "File Browser" -> "CPHDRIVE"
#  22. frontend/public/index.html        <title> fallback -> CPHDRIVE
#  23. frontend/src/views/settings/Global.vue
#      Dropped the two upstream GitHub doc links (custom branding, hook
#      runner) and the {2} placeholder they filled. The copy no longer names
#      the upstream project.
#  24. frontend/src/i18n/en.json
#      brandingHelp / tusUploadsHelp / executeOnShellDescription renamed to
#      CPHDRIVE; commandRunnerHelp lost its docs placeholder; the unused
#      sunset notice keys and the now-unused "documentation" key removed.
#  25. frontend/src/components/CustomToast.vue + frontend/src/main.ts
#      The error toast carried a "Report issue" button that opened the
#      upstream GitHub issue tracker. Removed, along with the displayReport
#      argument to $showError that only fed it.
#  26. frontend/src/components/Sidebar.vue + frontend/src/components/prompts/About.vue
#      The version suffix is hidden when the build reports "(untracked)",
#      which is what a local go build without ldflags says.
#  27. branding/img/icons/
#      CPHDRIVE favicon.svg, favicon.ico, apple-touch-icon.png and the two
#      Android icons, rendered from logo.svg. Served as runtime overrides
#      because static.go falls back to the embedded asset only when the file
#      is absent from branding.files. No rebuild needed to change them.
#
#   Header geometry
#  28. frontend/src/css/apple.css
#      The header is position: fixed to the viewport, so it does not inherit
#      main's left offset. header .logo is sized to var(--rail) + 1em, which
#      puts the search box on the same x as the breadcrumbs and the listing.
#      custom.css paints every header action with display: inline-flex
#      !important, which kills header.css's display: none for the burger and
#      the overflow dots; both are hidden again at >=737px with !important,
#      scoped so the phone drawer keeps them.
#
#   Drawer scrim
#  29. frontend/src/components/Sidebar.vue + frontend/src/css/apple.css
#      The drawer scrim is a full-screen backdrop-filter. Left open while the
#      window grew past 737px it kept blurring and blocking the whole desktop
#      app, because nothing closed the "sidebar" prompt on resize. The scrim
#      now carries .sidebar-scrim, is display: none above the breakpoint, and
#      a resize listener drops the drawer state when the rail becomes
#      permanent. The class matters: .overlay is shared with the header
#      dropdown and the share page, which must keep their own behaviour.
#
#   Search and view modes
#  30. frontend/src/css/apple.css + frontend/src/components/Search.vue
#      The search box filled the header's full content height and ran to 34em,
#      which read as a slab. It is now 2.5em tall, centred, and capped at
#      24em. The same rule also exposed a live bug: custom.css puts a
#      backdrop-filter on <header>, which makes it the containing block for
#      #search.active, so the full-screen search collapsed to the header's
#      59px and the page showed through under it. The element-level filter is
#      forced off; the blur stays on header::before.
#      The empty-search type filters were four 10em blue slabs with a 3.5em
#      icon. They are 136x48 chips now, four-up on desktop and two-up under
#      600px, and Search.vue hides the redundant "Type to search..." line
#      while they are on screen. custom.css's pill border and focus ring are
#      dropped while the overlay is open, where they stretch across the
#      window.
#      The two mosaic modes had become identical because custom.css gave both
#      the same 184px grid and 56px icon box. Gallery now uses 280px columns,
#      a 148px preview box, a 72px icon and a visible date; mosaic stays a
#      dense two-up grid on phones while gallery is one big card per row.
#
#   Search behaviour
#  31. frontend/src/components/Search.vue + frontend/src/css/apple.css
#      frontend/src/i18n/en.json
#      custom.css also forces the header's search button to inline-flex, so
#      the desktop showed the field and a second magnifier for the same
#      action. Hidden at >=737px; the phone still needs it because the field
#      is not in the phone header.
#      Search runs on a 250ms debounce while typing instead of waiting for
#      Enter, which still fires immediately. Each keystroke aborts the
#      in-flight request. The empty state says "No results" once a search
#      finishes with nothing, and the spinner covers the debounce window.
#
# The visual design is NOT inside the binary. It is served at runtime from the
# directory named by the database setting branding.files (on the server:
# /root/filebrowser-branding, holding custom.css and img/logo.svg). A copy
# lives in ./branding for local previews; that copy is set on the preview DB
# with:
#   filebrowser -d ./preview.db config set --branding.files <abs path>
# Keep the two copies in sync, or point branding.files at one place only.
#
# Gotcha for anyone touching the header: its backdrop-filter lives on
# header::before, not on <header>. backdrop-filter makes an element the
# containing block for position: fixed descendants, and #search.active plus
# #dropdown are both fixed children of the header. Blurring the header itself
# collapsed the full-screen search overlay to the header's own height.
#
# Not applied: the shipped binary had frontend/public/index.html minified.
# This tree does not minify it, so the output is ~1.5 KB larger. Behaviour is
# identical. Add an HTML minify step to the vite build if byte parity matters.
#
# Note: the original was patched by rewriting bytes inside the embedded JS
# bundle and its pre-compressed .gz twin. A normal build regenerates both
# from source, so that step is not needed here.
#
# Requirements: Go 1.26.5 at /c/dev/go (override with GO=...), Node >= 24.
#
# Deploy after building:
#   scp filebrowser-linux-386 cronzph:~/fb-new
#   ssh cronzph 'sudo install -m 755 ~/fb-new /usr/local/bin/filebrowser \
#                && sudo systemctl restart filebrowser'

set -euo pipefail

cd "$(dirname "$0")"

VERSION=2.63.23
COMMIT=e8a388f8
GO=${GO:-/c/dev/go/bin/go}

echo "==> frontend"
cd frontend
npx --yes pnpm@10 install --frozen-lockfile
npx --yes pnpm@10 build
cd ..

echo "==> backend (linux/386)"
LDFLAGS="-s -w -X github.com/filebrowser/filebrowser/v2/version.Version=$VERSION -X github.com/filebrowser/filebrowser/v2/version.CommitSHA=$COMMIT"
CGO_ENABLED=0 GOOS=linux GOARCH=386 "$GO" build -ldflags "$LDFLAGS" -o filebrowser-linux-386 .

echo "==> done"
ls -l filebrowser-linux-386
