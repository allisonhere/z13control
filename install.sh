#!/usr/bin/env bash
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GO_CACHE_DIR="${GO_CACHE_DIR:-/tmp/z13center-go-cache}"
GO_MOD_CACHE_DIR="${GO_MOD_CACHE_DIR:-/tmp/z13center-go-mod-cache}"
INSTALL_PREFIX="${INSTALL_PREFIX:-/usr/local}"
INSTALL_BINDIR="${INSTALL_BINDIR:-$INSTALL_PREFIX/bin}"
INSTALL_APPDIR="${INSTALL_APPDIR:-$INSTALL_PREFIX/share/applications}"
INSTALL_UDEVDIR="${INSTALL_UDEVDIR:-/etc/udev/rules.d}"
INSTALL_USERUNITDIR="${INSTALL_USERUNITDIR:-/usr/lib/systemd/user}"
UPDATE_REMOTE="${UPDATE_REMOTE:-origin}"
UPDATE_BRANCH="${UPDATE_BRANCH:-main}"
STEP_START=0
FLOW_STEP=0
FLOW_TOTAL=0

print_header() {
  if [ -t 1 ] && command -v clear >/dev/null 2>&1; then
    clear
  fi
  echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
  echo -e "${BLUE}║${NC}                  ${BOLD}${CYAN}z13center Installer${NC}                      ${BLUE}║${NC}"
  echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
  echo ""
}

print_menu_box() {
  echo -e "${BLUE}┌──────────────────────────────────────────────────────────┐${NC}"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "Project: $PROJECT_DIR"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "Update source: $UPDATE_REMOTE/$UPDATE_BRANCH"
  echo -e "${BLUE}├──────────────────────────────────────────────────────────┤${NC}"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "1) Install z13center             (recommended)"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "2) Update an existing install"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "3) Check installation status"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "4) Repair / reinstall"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "5) Advanced actions"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "6) Uninstall z13center"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "q) Quit"
  echo -e "${BLUE}├──────────────────────────────────────────────────────────┤${NC}"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "Press Enter to choose the recommended install."
  echo -e "${BLUE}└──────────────────────────────────────────────────────────┘${NC}"
}

print_step() {
  STEP_START=$(date +%s)
  echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${BOLD}$1${NC}"
  echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

print_substep() { echo -e "  ${DIM}→${NC} $1"; }
print_info() { echo -e "  ${BLUE}ℹ${NC} $1"; }
print_warn() { echo -e "  ${YELLOW}⚠${NC} $1"; }
print_error() { echo -e "  ${RED}✗${NC} $1" >&2; }
print_success() {
  local elapsed
  elapsed=$(($(date +%s) - STEP_START))
  echo -e "  ${GREEN}✓${NC} $1 ${DIM}(${elapsed}s)${NC}"
}

begin_flow() {
  FLOW_STEP=0
  FLOW_TOTAL=$1
}

flow_step() {
  FLOW_STEP=$((FLOW_STEP + 1))
  echo ""
  echo -e "${BOLD}${CYAN}[$FLOW_STEP/$FLOW_TOTAL] $1${NC}"
}

run_cmd() {
  echo -e "  ${DIM}$*${NC}"
  "$@"
}

require_command() {
  local name=$1
  if ! command -v "$name" >/dev/null 2>&1; then
    print_error "Missing required command: $name"
    exit 1
  fi
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

current_branch() {
  git -C "$PROJECT_DIR" branch --show-current
}

dirty_count() {
  git -C "$PROJECT_DIR" status --porcelain | wc -l | tr -d ' '
}

confirm() {
  local prompt=$1
  local answer
  read -r -p "$prompt [y/N]: " answer
  case "$answer" in
    y|Y|yes|YES) return 0 ;;
    *) return 1 ;;
  esac
}

pause_for_input() {
  echo ""
  read -r -p "Press Enter to continue..." _
}

run_action() {
  local action=$1
  local status
  shift
  echo ""
  set +e
  (
    set -e
    "$action" "$@"
  )
  status=$?
  set -e
  if [ "$status" -eq 0 ]; then
    echo ""
    echo -e "${GREEN}${BOLD}Done.${NC}"
  elif [ "$status" -eq 10 ]; then
    :
  else
    echo ""
    print_error "The action could not be completed (exit code $status)."
    print_info "The remaining steps were stopped. Review the message above, then try again."
  fi
  pause_for_input
}

ensure_repo() {
  if [ ! -f "$PROJECT_DIR/Makefile" ] || [ ! -f "$PROJECT_DIR/main.go" ]; then
    print_error "This does not look like the z13center project root"
    exit 1
  fi
}

check_prereqs() {
  local missing=0 service_state="not installed" daemon_state="not installed"
  local tool_name module_name
  print_step "Installation status"
  ensure_repo

  print_info "Installed components"
  if [ -x "$INSTALL_BINDIR/z13center" ]; then
    echo -e "  - Desktop binary: ${GREEN}installed${NC} ($INSTALL_BINDIR/z13center)"
  else
    echo -e "  - Desktop binary: ${YELLOW}not installed${NC}"
  fi
  if [ -x "$HOME/.local/bin/z13center" ]; then
    echo -e "  - User binary: ${GREEN}installed${NC} ($HOME/.local/bin/z13center)"
  else
    echo -e "  - User binary: ${YELLOW}not installed${NC}"
  fi
  if has_command systemctl; then
    service_state=$(systemctl --user is-active z13center.service 2>/dev/null || true)
    daemon_state=$(systemctl --user is-active z13ctl.service 2>/dev/null || true)
    [ -n "$service_state" ] || service_state="unavailable"
    [ -n "$daemon_state" ] || daemon_state="unavailable"
  fi
  echo "  - z13center service: $service_state"
  echo "  - z13ctl hardware service: $daemon_state"
  if [ -f "$INSTALL_UDEVDIR/70-z13-aura-uaccess.rules" ]; then
    echo -e "  - Hardware permission rule: ${GREEN}installed${NC}"
  else
    echo -e "  - Hardware permission rule: ${YELLOW}not installed${NC}"
  fi
  echo ""

  print_info "Required tools"
  for tool_name in make go pkg-config sudo systemctl udevadm; do
    if has_command "$tool_name"; then
      echo -e "  - $tool_name: ${GREEN}ready${NC}"
    else
      echo -e "  - $tool_name: ${RED}missing${NC}"
      missing=$((missing + 1))
    fi
  done
  if has_command go; then
    echo "  - Go version: $(go env GOVERSION 2>/dev/null || go version)"
  fi
  if has_command pkg-config; then
    for module_name in gtk4 graphene-gobject-1.0 gobject-introspection-1.0; do
      if pkg-config --exists "$module_name"; then
        echo -e "  - $module_name headers: ${GREEN}ready${NC}"
      else
        echo -e "  - $module_name headers: ${RED}missing${NC}"
        missing=$((missing + 1))
      fi
    done
  fi
  if has_command git; then
    echo -e "  - git: ${GREEN}ready${NC} (needed for updates)"
  else
    echo -e "  - git: ${YELLOW}missing${NC} (updates unavailable)"
  fi
  if has_command ssh; then
    echo -e "  - ssh: ${GREEN}ready${NC} (used by SSH remotes)"
  else
    echo -e "  - ssh: ${YELLOW}missing${NC} (HTTPS remotes still work)"
  fi
  echo ""

  if has_command git; then
    print_info "Git state"
    echo "  - Branch: $(current_branch)"
    echo "  - Pending changes: $(dirty_count)"
    echo "  - Remote: $UPDATE_REMOTE/$UPDATE_BRANCH"
    echo ""
  fi

  if [ "$missing" -gt 0 ]; then
    print_warn "$missing required tool(s) are missing."
    if [ -f /etc/arch-release ]; then
      print_info "Arch: sudo pacman -S --needed base-devel go gtk4 graphene gobject-introspection pkgconf systemd"
    elif has_command apt-get; then
      print_info "Debian/Ubuntu: sudo apt install build-essential golang-go libgtk-4-dev libgraphene-1.0-dev libgirepository1.0-dev pkg-config systemd udev"
    elif has_command dnf; then
      print_info "Fedora: sudo dnf install golang make gcc pkgconf-pkg-config gtk4-devel graphene-devel gobject-introspection-devel systemd-udev"
    fi
    print_info "Install the missing tools, then run this check again."
    return 0
  fi

  if [ "$daemon_state" = "not installed" ] || [ "$daemon_state" = "inactive" ]; then
    print_warn "z13ctl is required for live hardware controls."
  fi
  print_success "System is ready"
}

verify_install_tools() {
  local tool_name module_name missing=0
  for tool_name in make go pkg-config sudo systemctl udevadm; do
    if ! has_command "$tool_name"; then
      print_error "Missing required tool: $tool_name"
      missing=1
    fi
  done
  if has_command pkg-config; then
    for module_name in gtk4 graphene-gobject-1.0 gobject-introspection-1.0; do
      if ! pkg-config --exists "$module_name"; then
        print_error "Missing development headers: $module_name"
        missing=1
      fi
    done
  fi
  if [ "$missing" -ne 0 ]; then
    print_info "Choose 'Check installation status' for package suggestions."
    return 1
  fi
}

update_existing_install() {
  local branch local_sha remote_sha base_sha remote_url transport="HTTPS" updated=0
  print_step "Update existing install"
  ensure_repo
  require_command git
  verify_install_tools
  begin_flow 7

  flow_step "Validate the local checkout"
  remote_url=$(git -C "$PROJECT_DIR" remote get-url "$UPDATE_REMOTE")
  case "$remote_url" in
    git@*|ssh://*)
      require_command ssh
      transport="SSH"
      ;;
  esac
  branch=$(current_branch)
  if [ "$branch" != "$UPDATE_BRANCH" ]; then
    print_error "Current branch is $branch, expected $UPDATE_BRANCH"
    print_info "Override with UPDATE_BRANCH=$branch if this is intentional"
    exit 1
  fi

  if [ "$(dirty_count)" -ne 0 ]; then
    print_error "Working tree is dirty. Commit, stash, or discard local changes before updating."
    exit 1
  fi

  flow_step "Check GitHub for updates"
  print_substep "Fetching $UPDATE_REMOTE/$UPDATE_BRANCH over $transport"
  (
    cd "$PROJECT_DIR"
    run_cmd git fetch "$UPDATE_REMOTE" "$UPDATE_BRANCH"
  )

  local_sha=$(git -C "$PROJECT_DIR" rev-parse HEAD)
  remote_sha=$(git -C "$PROJECT_DIR" rev-parse "$UPDATE_REMOTE/$UPDATE_BRANCH")
  base_sha=$(git -C "$PROJECT_DIR" merge-base HEAD "$UPDATE_REMOTE/$UPDATE_BRANCH")

  echo "  - Local:  ${local_sha:0:12}"
  echo "  - Remote: ${remote_sha:0:12}"

  if [ "$local_sha" = "$remote_sha" ]; then
    print_info "Local checkout is already up to date"
  else
    if [ "$base_sha" != "$local_sha" ]; then
      print_error "Local branch has commits not on $UPDATE_REMOTE/$UPDATE_BRANCH"
      print_info "Rebase or merge manually before using the updater"
      exit 1
    fi

    print_substep "Fast-forwarding to $UPDATE_REMOTE/$UPDATE_BRANCH"
    (
      cd "$PROJECT_DIR"
      run_cmd git pull --ff-only "$UPDATE_REMOTE" "$UPDATE_BRANCH"
    )
    updated=1
  fi

  flow_step "Build the updated application"
  build_app
  flow_step "Install desktop files and hardware permissions"
  install_system_files
  flow_step "Install the per-user service"
  install_user_service

  flow_step "Start the application service"
  if confirm "Enable or restart z13center.service now?"; then
    enable_user_service
  else
    print_info "Skipped z13center.service restart"
  fi

  flow_step "Restart the hardware service"
  if confirm "Restart z13ctl.service now?"; then
    restart_z13ctl_service
  else
    print_info "Skipped z13ctl.service restart"
  fi

  if [ "$updated" -eq 1 ]; then
    print_success "Update complete from $UPDATE_REMOTE/$UPDATE_BRANCH"
  else
    print_success "Reinstall complete; repo was already current"
  fi
}

build_app() {
  print_step "Build z13center"
  ensure_repo
  require_command make
  require_command go
  (
    cd "$PROJECT_DIR"
    run_cmd env GOCACHE="$GO_CACHE_DIR" GOMODCACHE="$GO_MOD_CACHE_DIR" make clean build
  )
  print_success "Binary built at $PROJECT_DIR/z13center"
}

install_system_files() {
  print_step "Install system files"
  ensure_repo
  require_command sudo
  require_command make

  print_info "This step will install:"
  echo "  - z13center to $INSTALL_BINDIR"
  echo "  - z13center.desktop to $INSTALL_APPDIR"
  echo "  - z13center.service to $INSTALL_USERUNITDIR"
  echo "  - 70-z13-aura-uaccess.rules to $INSTALL_UDEVDIR"
  echo ""

  (
    cd "$PROJECT_DIR"
    run_cmd sudo env GOCACHE="$GO_CACHE_DIR" GOMODCACHE="$GO_MOD_CACHE_DIR" \
      make PREFIX="$INSTALL_PREFIX" BINDIR="$INSTALL_BINDIR" APPDIR="$INSTALL_APPDIR" \
      UDEVDIR="$INSTALL_UDEVDIR" USERUNITDIR="$INSTALL_USERUNITDIR" install
  )
  print_success "System files installed"
}

install_user_service() {
  print_step "Install per-user service"
  ensure_repo
  require_command make
  require_command systemctl

  print_info "This step installs the user service to ~/.config/systemd/user and the binary to ~/.local/bin."
  (
    cd "$PROJECT_DIR"
    run_cmd env GOCACHE="$GO_CACHE_DIR" GOMODCACHE="$GO_MOD_CACHE_DIR" make install-user
  )
  print_success "Per-user service installed"
}

enable_user_service() {
  print_step "Enable and start z13center.service"
  require_command systemctl
  run_cmd systemctl --user daemon-reload
  run_cmd systemctl --user enable --now z13center.service
  print_success "z13center.service enabled and started"
}

restart_z13ctl_service() {
  print_step "Restart z13ctl.service"
  require_command systemctl
  run_cmd systemctl --user restart z13ctl.service
  print_success "z13ctl.service restarted"
}

uninstall_system_files() {
  print_step "Uninstall system files"
  require_command sudo
  require_command make
  (
    cd "$PROJECT_DIR"
    run_cmd sudo make PREFIX="$INSTALL_PREFIX" BINDIR="$INSTALL_BINDIR" APPDIR="$INSTALL_APPDIR" \
      UDEVDIR="$INSTALL_UDEVDIR" USERUNITDIR="$INSTALL_USERUNITDIR" uninstall
  )
  print_success "System files removed"
}

remove_user_service() {
  print_step "Remove per-user service"
  require_command systemctl
  systemctl --user disable --now z13center.service >/dev/null 2>&1 || true
  run_cmd rm -f "$HOME/.config/systemd/user/z13center.service"
  run_cmd rm -f "$HOME/.local/bin/z13center"
  run_cmd systemctl --user daemon-reload
  print_success "Per-user service removed"
}

full_install() {
  local confirmed=${1:-no}
  print_step "Full guided install"
  print_info "This installs the desktop launcher, hardware permission rule, and background user service."
  print_info "The desktop and service copies are both needed because the service runs from ~/.local/bin."
  echo ""
  if [ "$confirmed" != "yes" ] && ! confirm "Continue with the recommended installation?"; then
    print_info "Installation cancelled"
    return 10
  fi
  begin_flow 6
  flow_step "Check requirements"
  verify_install_tools
  print_substep "Requesting administrator access now so it does not interrupt a later step"
  run_cmd sudo -v
  flow_step "Build the application"
  build_app
  flow_step "Install desktop files and hardware permissions"
  install_system_files
  flow_step "Install the per-user service"
  install_user_service
  flow_step "Start the application service"
  if confirm "Enable and start z13center.service now?"; then
    enable_user_service
  else
    print_info "Skipped enabling z13center.service"
  fi
  flow_step "Restart the hardware service"
  if confirm "Restart z13ctl.service now?"; then
    restart_z13ctl_service
  else
    print_info "Skipped z13ctl.service restart"
  fi
  print_success "Full install flow complete"
}

repair_install() {
  print_step "Repair / reinstall z13center"
  print_info "This rebuilds and replaces installed files without changing your settings."
  echo ""
  if ! confirm "Continue with repair?"; then
    print_info "Repair cancelled"
    return 10
  fi
  full_install yes
}

uninstall_all() {
  print_step "Uninstall z13center"
  print_warn "This removes the app, desktop entry, permission rule, and user service."
  print_info "Your source checkout and z13ctl installation will not be removed."
  echo ""
  if ! confirm "Remove z13center from this computer?"; then
    print_info "Uninstall cancelled"
    return 10
  fi
  require_command sudo
  print_substep "Requesting administrator access before removing anything"
  run_cmd sudo -v
  begin_flow 2
  flow_step "Remove the user service"
  if systemctl --user list-unit-files z13center.service >/dev/null 2>&1 ||
     [ -f "$HOME/.config/systemd/user/z13center.service" ]; then
    remove_user_service
  else
    print_info "User service is not installed; skipping"
  fi
  flow_step "Remove system files"
  uninstall_system_files
  print_success "z13center has been uninstalled"
}

advanced_menu() {
  local choice
  while true; do
    print_header
    echo -e "${BLUE}┌──────────────────────────────────────────────────────────┐${NC}"
    printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "Advanced actions"
    echo -e "${BLUE}├──────────────────────────────────────────────────────────┤${NC}"
    printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "1) Build only"
    printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "2) Install system files + permission rule"
    printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "3) Install per-user service files"
    printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "4) Enable / restart z13center.service"
    printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "5) Restart z13ctl.service"
    printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "b) Back"
    echo -e "${BLUE}└──────────────────────────────────────────────────────────┘${NC}"
    echo ""
    read -r -p "Select action: " choice
    case "$choice" in
      1) run_action build_app ;;
      2) run_action install_system_files ;;
      3) run_action install_user_service ;;
      4) run_action enable_user_service ;;
      5) run_action restart_z13ctl_service ;;
      b|B) return 0 ;;
      *) print_error "Please choose 1-5 or b"; pause_for_input ;;
    esac
  done
}

run_menu() {
  local choice
  while true; do
    print_header
    print_menu_box
    echo ""
    read -r -p "Select action [1]: " choice

    case "$choice" in
      1|"") run_action full_install ;;
      2) run_action update_existing_install ;;
      3) run_action check_prereqs ;;
      4) run_action repair_install ;;
      5) advanced_menu ;;
      6) run_action uninstall_all ;;
      q|Q) exit 0 ;;
      *) print_error "Please choose 1-6 or q"; pause_for_input ;;
    esac
  done
}

main() {
  ensure_repo
  run_menu
}

main "$@"
