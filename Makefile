# z13center — modern GTK4 control center for the ASUS ROG Flow Z13 (2025).

BINARY  := z13center
PREFIX  ?= /usr/local
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.Version=$(VERSION)
GO      ?= go

UDEV_RULE := 70-z13-aura-uaccess.rules
BINDIR   ?= $(PREFIX)/bin
APPDIR   ?= $(PREFIX)/share/applications
UDEVDIR  ?= /etc/udev/rules.d
USERUNITDIR ?= /usr/lib/systemd/user

.PHONY: all build run clean install uninstall install-user install-udev uninstall-udev fmt vet tidy

all: build

build:
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BINARY) .

run: build
	./$(BINARY)

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

clean:
	rm -f $(BINARY)

install: build install-udev
	install -Dm755 $(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	install -Dm644 contrib/z13center.desktop $(DESTDIR)$(APPDIR)/z13center.desktop
	install -Dm644 contrib/z13center.service $(DESTDIR)$(USERUNITDIR)/z13center.service

uninstall: uninstall-udev
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY)
	rm -f $(DESTDIR)$(APPDIR)/z13center.desktop
	rm -f $(DESTDIR)$(USERUNITDIR)/z13center.service

# Install the uaccess udev rule so RGB lighting works for the logged-in user,
# regardless of username or group. Requires root; reloads + re-triggers udev.
install-udev:
	install -Dm644 contrib/$(UDEV_RULE) $(DESTDIR)$(UDEVDIR)/$(UDEV_RULE)
ifeq ($(DESTDIR),)
	udevadm control --reload
	udevadm trigger --subsystem-match=hidraw
	@echo "udev rule installed. Restart the daemon: systemctl --user restart z13ctl.service"
endif

uninstall-udev:
	rm -f $(DESTDIR)$(UDEVDIR)/$(UDEV_RULE)

# Install + enable the per-user systemd service (runs alongside the z13ctl daemon).
install-user: build
	install -Dm755 $(BINARY) $(HOME)/.local/bin/$(BINARY)
	install -Dm644 contrib/z13center.service $(HOME)/.config/systemd/user/z13center.service
	systemctl --user daemon-reload
	@echo "Enable with: systemctl --user enable --now z13center.service"
