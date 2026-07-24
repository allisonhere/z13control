# z13center

A modern **GTK4 control center** for the 2025 ASUS ROG Flow Z13 (GZ302EA).

Where [z13gui](https://github.com/dahui/z13gui) is a minimal touch-friendly edge
drawer, **z13center** is a full-window desktop dashboard: live telemetry gauges,
a draggable fan-curve editor, colour-zoned power controls, and a dark ROG-neon
theme. It drives the same hardware through the same
[`z13ctl`](https://github.com/dahui/z13ctl) daemon.

## Features

- **Dashboard** — live APU-temperature and fan-RPM ring gauges (eased animation,
  1 Hz), big performance-profile cards, and an at-a-glance status strip.
- **Fan curve** — draggable 8-point editor on a cairo canvas with a dashed
  live-temperature marker; constraints keep temps increasing and PWM monotonic.
- **Power** — a safe basic TDP slider (5–70 W) plus an Advanced mode exposing
  independent PL1/PL2/PL3 up to 93 W with force-mode warnings, and a CPU
  Curve-Optimizer undervolt slider that hides itself when `ryzen_smu` is absent.
- **Lighting** — per-zone (keyboard / lightbar / all) Aura effects, colour
  swatches, brightness, and custom 2–8 color palette cycles with a visual
  gradient editor and slow/normal/fast playback.
- **Battery & display** — charge-limit slider, panel overdrive, POST boot sound.

## How it talks to the hardware

All hardware access goes through the `z13ctl` daemon over its Unix socket via the
published [`github.com/dahui/z13ctl/api`](https://pkg.go.dev/github.com/dahui/z13ctl/api)
client. When the daemon isn't running, z13center falls back to an in-memory
**preview mode** — the whole UI stays interactive and shows an "offline" banner,
so it's usable for development without the daemon or even the hardware.

## Build

Requires Go 1.25+ and the GTK4 development headers:

```sh
# Fedora
sudo dnf install gtk4-devel graphene-devel glib2-devel gobject-introspection-devel \
                 cairo-gobject-devel pango-devel

make build      # -> ./z13center
make run        # build + launch
```

For live control (not preview), install and enable the
[`z13ctl` daemon](https://github.com/dahui/z13ctl) first.

Custom palette cycles require a daemon that advertises the `palette-cycle`
capability. Until that support is released upstream, apply
[`contrib/z13ctl-palette-cycle.patch`](contrib/z13ctl-palette-cycle.patch) to a
z13ctl checkout and rebuild it.

### RGB lighting permissions

The daemon reaches the keyboard/lightbar through `hidraw` nodes. `z13ctl setup`
grants these to the `users` group, which works on Arch but not on distros (e.g.
Fedora) where regular users aren't in `users`. z13center ships a portable
`uaccess` udev rule (`contrib/70-z13-aura-uaccess.rules`) that grants the
**logged-in user** access via a dynamic ACL — no group membership, no hardcoded
username, works for any user. `sudo make install` installs it automatically, or
just the rule:

```sh
sudo make install-udev
systemctl --user restart z13ctl.service
```

## Install

### Guided install (recommended)

Clone the repository, then start the installer:

```sh
# SSH
git clone git@github.com:allisonhere/z13control.git
cd z13control

# Start the installer
./install.sh
```

If you do not use a GitHub SSH key, clone over HTTPS instead:

```sh
git clone https://github.com/allisonhere/z13control.git
cd z13control
./install.sh
```

Press **Enter** at the menu to select the recommended installation. The
installer will:

- check Go, GTK4, and other build requirements
- build z13center
- install the application and desktop launcher
- install the RGB hardware-permission rule
- install and optionally start the per-user service
- optionally restart `z13ctl.service`

The installer also provides Status, Repair, Advanced, and Uninstall actions. It
explains each change before making it and shows numbered progress during
multi-step operations.

### Updating

Run the installer again and choose **Update an existing install**:

```sh
cd z13control
./install.sh
```

The updater checks `origin/main` over SSH or HTTPS, fast-forwards the local
checkout, rebuilds the application, reinstalls its files, and offers to restart
the services. Commit or stash local changes before updating.

### Manual install

To install without the guided installer:

```sh
make build
sudo make install
make install-user
systemctl --user enable --now z13center.service
systemctl --user restart z13ctl.service
```

## Packaging

Arch and Debian packaging scaffolding lives in-repo:

- `pkg/arch/PKGBUILD` and `pkg/arch/.SRCINFO` for an AUR-style `z13center-git`
  package
- `debian/` for standard Debian package builds

Typical package build entrypoints:

```sh
# Arch
cd pkg/arch
makepkg --syncdeps --cleanbuild

# Debian
dpkg-buildpackage -us -uc
```

Both package families install:

- the `z13center` binary
- the desktop launcher
- the user systemd unit
- the `uaccess` RGB `udev` rule

The packages do not bundle `z13ctl`. Install it separately for live hardware
control.

## License

Apache-2.0, matching the upstream z13ctl / z13gui projects.
