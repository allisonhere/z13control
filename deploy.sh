#!/usr/bin/env bash
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_REMOTE="${REPO_REMOTE:-origin}"
REPO_BRANCH="${REPO_BRANCH:-main}"
DEFAULT_COMMIT_MESSAGE="${DEPLOY_COMMIT_MESSAGE:-chore: release}"
VERSION_PREFIX="${VERSION_PREFIX:-v}"
GO_CACHE_DIR="${GO_CACHE_DIR:-/tmp/z13center-go-cache}"
GO_MOD_CACHE_DIR="${GO_MOD_CACHE_DIR:-/tmp/z13center-go-mod-cache}"
RELEASE_DIR="${RELEASE_DIR:-$PROJECT_DIR/dist/release}"
STEP_START=0
RELEASE_VERSION=""

print_header() {
  if [ -t 1 ] && command -v clear >/dev/null 2>&1; then
    clear
  fi
  echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
  echo -e "${BLUE}║${NC}                  ${BOLD}${CYAN}z13center Release TUI${NC}                    ${BLUE}║${NC}"
  echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
  echo ""
}

print_menu_box() {
  local latest changes selected
  latest=$(latest_tag)
  changes=$(dirty_count)
  selected=${RELEASE_VERSION:-none}

  echo -e "${BLUE}┌──────────────────────────────────────────────────────────┐${NC}"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "Branch: $(current_branch)"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "Remote: $REPO_REMOTE/$REPO_BRANCH"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "Latest tag: ${latest:-none}"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "Selected version: $selected"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "Pending changes: $changes"
  echo -e "${BLUE}├──────────────────────────────────────────────────────────┤${NC}"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "1) Status"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "2) Preflight build + test"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "3) Commit changes"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "4) Push branch"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "5) Commit + push code only"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "6) Select / bump version tag"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "7) Build binary"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "8) Build Arch package"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "9) Build Debian package"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "10) Full release"
  printf "${BLUE}│${NC} %-56s ${BLUE}│${NC}\n" "q) Quit"
  echo -e "${BLUE}└──────────────────────────────────────────────────────────┘${NC}"
}

print_step() {
  STEP_START=$(date +%s)
  echo -e "${MAGENTA}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${BOLD}$1${NC}"
  echo -e "${MAGENTA}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
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

current_branch() {
  git -C "$PROJECT_DIR" branch --show-current
}

dirty_count() {
  git -C "$PROJECT_DIR" status --porcelain | wc -l | tr -d ' '
}

latest_tag() {
  git -C "$PROJECT_DIR" tag --sort=-version:refname | awk -v prefix="$VERSION_PREFIX" '
    index($0, prefix) == 1 && $0 ~ ("^" prefix "[0-9]+\\.[0-9]+\\.[0-9]+$") { print; exit }'
}

normalize_version() {
  local raw=$1
  raw="${raw#${VERSION_PREFIX}}"
  echo "$raw"
}

version_components() {
  local raw
  raw=$(normalize_version "$1")
  IFS=. read -r VERSION_MAJOR VERSION_MINOR VERSION_PATCH <<<"$raw"
}

next_version_for_bump() {
  local bump=$1
  local base=${2:-}

  if [ -z "$base" ]; then
    case "$bump" in
      patch) echo "${VERSION_PREFIX}0.1.0" ;;
      minor) echo "${VERSION_PREFIX}0.1.0" ;;
      major) echo "${VERSION_PREFIX}1.0.0" ;;
      *) print_error "Unknown bump type: $bump"; exit 1 ;;
    esac
    return
  fi

  version_components "$base"
  case "$bump" in
    patch) VERSION_PATCH=$((VERSION_PATCH + 1)) ;;
    minor) VERSION_MINOR=$((VERSION_MINOR + 1)); VERSION_PATCH=0 ;;
    major) VERSION_MAJOR=$((VERSION_MAJOR + 1)); VERSION_MINOR=0; VERSION_PATCH=0 ;;
    *) print_error "Unknown bump type: $bump"; exit 1 ;;
  esac
  echo "${VERSION_PREFIX}${VERSION_MAJOR}.${VERSION_MINOR}.${VERSION_PATCH}"
}

ensure_repo() {
  if ! git -C "$PROJECT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    print_error "$PROJECT_DIR is not a Git repository"
    exit 1
  fi
}

ensure_branch() {
  local branch
  branch=$(current_branch)
  if [ "$branch" != "$REPO_BRANCH" ]; then
    print_error "Expected branch $REPO_BRANCH but current branch is $branch"
    print_info "Override with REPO_BRANCH=$branch if this is intentional"
    exit 1
  fi
}

ensure_release_dir() {
  mkdir -p "$RELEASE_DIR"
}

status_report() {
  local branch changes latest upstream
  branch=$(current_branch)
  changes=$(dirty_count)
  latest=$(latest_tag)
  upstream=$(git -C "$PROJECT_DIR" rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null || echo "none")

  echo -e "  ${BOLD}Project:${NC}     $PROJECT_DIR"
  echo -e "  ${BOLD}Branch:${NC}      $branch"
  echo -e "  ${BOLD}Upstream:${NC}    $upstream"
  echo -e "  ${BOLD}Remote:${NC}      $REPO_REMOTE/$REPO_BRANCH"
  echo -e "  ${BOLD}Latest tag:${NC}  ${latest:-none}"
  echo -e "  ${BOLD}Pending:${NC}     $changes change(s)"
  echo -e "  ${BOLD}Release dir:${NC} $RELEASE_DIR"
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

choose_bump() {
  local latest patch minor major choice custom
  latest=$(latest_tag)
  patch=$(next_version_for_bump patch "$latest")
  minor=$(next_version_for_bump minor "$latest")
  major=$(next_version_for_bump major "$latest")

  echo ""
  echo "Select release version:"
  echo "  1) patch  -> $patch"
  echo "  2) minor  -> $minor"
  echo "  3) major  -> $major"
  echo "  4) custom"
  read -r -p "Choice [1-4]: " choice

  case "$choice" in
    1|"") RELEASE_VERSION="$patch" ;;
    2) RELEASE_VERSION="$minor" ;;
    3) RELEASE_VERSION="$major" ;;
    4)
      read -r -p "Enter version (${VERSION_PREFIX}X.Y.Z): " custom
      if [[ ! "$custom" =~ ^${VERSION_PREFIX}[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        print_error "Version must match ${VERSION_PREFIX}X.Y.Z"
        exit 1
      fi
      RELEASE_VERSION="$custom"
      ;;
    *)
      print_error "Invalid choice"
      exit 1
      ;;
  esac

  if git -C "$PROJECT_DIR" rev-parse "$RELEASE_VERSION" >/dev/null 2>&1; then
    print_error "Tag $RELEASE_VERSION already exists"
    exit 1
  fi
}

prepare_release_notes() {
  local notes_file=$1
  local default_body
  default_body=$(cat <<EOF
# $RELEASE_VERSION

## Summary

- 

## Packaging

- Arch: AUR-style package metadata in \`pkg/arch/\`
- Debian: native packaging in \`debian/\`

## Notes

- 
EOF
)
  printf '%s\n' "$default_body" >"$notes_file"
  "${EDITOR:-nano}" "$notes_file"
}

preflight() {
  print_step "Preflight"
  require_command git
  require_command go
  require_command gh

  ensure_repo
  ensure_branch
  ensure_release_dir

  print_substep "Checking GitHub authentication"
  run_cmd gh auth status

  print_substep "Building z13center"
  (
    cd "$PROJECT_DIR"
    run_cmd env GOCACHE="$GO_CACHE_DIR" GOMODCACHE="$GO_MOD_CACHE_DIR" make clean build
  )

  print_substep "Running tests"
  (
    cd "$PROJECT_DIR"
    run_cmd env GOCACHE="$GO_CACHE_DIR" GOMODCACHE="$GO_MOD_CACHE_DIR" go test -buildvcs=false ./...
  )

  print_success "Preflight passed"
}

commit_changes() {
  local message
  if [ "$(dirty_count)" -eq 0 ]; then
    print_info "No uncommitted changes"
    return
  fi

  git -C "$PROJECT_DIR" status --short
  echo ""
  read -r -p "Commit message [$DEFAULT_COMMIT_MESSAGE]: " message
  message=${message:-$DEFAULT_COMMIT_MESSAGE}

  print_substep "Staging changes"
  (cd "$PROJECT_DIR" && run_cmd git add -A)

  if git -C "$PROJECT_DIR" diff --cached --quiet; then
    print_info "No staged changes after git add"
    return
  fi

  print_substep "Creating commit"
  (cd "$PROJECT_DIR" && run_cmd git commit -m "$message")
  print_success "Committed changes"
}

push_branch() {
  local local_sha remote_sha base_sha
  print_step "Push Branch"
  print_substep "Fetching remote state"
  (cd "$PROJECT_DIR" && run_cmd git fetch "$REPO_REMOTE" "$REPO_BRANCH")

  local_sha=$(git -C "$PROJECT_DIR" rev-parse HEAD)
  remote_sha=$(git -C "$PROJECT_DIR" rev-parse "$REPO_REMOTE/$REPO_BRANCH" 2>/dev/null || echo "")

  if [ -n "$remote_sha" ]; then
    base_sha=$(git -C "$PROJECT_DIR" merge-base HEAD "$REPO_REMOTE/$REPO_BRANCH")
    if [ "$base_sha" != "$remote_sha" ]; then
      print_error "$REPO_REMOTE/$REPO_BRANCH has commits this checkout does not have"
      exit 1
    fi
    if [ "$local_sha" = "$remote_sha" ]; then
      print_info "Branch already pushed"
      return
    fi
  fi

  print_substep "Pushing branch"
  (cd "$PROJECT_DIR" && run_cmd git push "$REPO_REMOTE" "$REPO_BRANCH")
  print_success "Branch pushed"
}

build_arch_package() {
  local output_dir=$1
  if ! command -v makepkg >/dev/null 2>&1; then
    print_warn "Skipping Arch package build: makepkg not installed"
    return
  fi

  print_step "Build Arch Package"
  mkdir -p "$output_dir"
  (
    cd "$PROJECT_DIR/pkg/arch"
    run_cmd env GOCACHE="$GO_CACHE_DIR" GOMODCACHE="$GO_MOD_CACHE_DIR" makepkg --force --cleanbuild --syncdeps --noconfirm
  )
  find "$PROJECT_DIR/pkg/arch" -maxdepth 1 -type f \( -name '*.pkg.tar.*' -o -name '*.tar.zst' \) -exec cp -f {} "$output_dir/" \;
  print_success "Arch package build finished"
}

build_debian_package() {
  local output_dir=$1
  if ! command -v dpkg-buildpackage >/dev/null 2>&1; then
    print_warn "Skipping Debian package build: dpkg-buildpackage not installed"
    return
  fi

  print_step "Build Debian Package"
  mkdir -p "$output_dir"
  (
    cd "$PROJECT_DIR"
    run_cmd env GOCACHE="$GO_CACHE_DIR" GOMODCACHE="$GO_MOD_CACHE_DIR" dpkg-buildpackage -us -uc
  )
  find "$(dirname "$PROJECT_DIR")" -maxdepth 1 -type f \( -name '*.deb' -o -name '*.buildinfo' -o -name '*.changes' \) -exec cp -f {} "$output_dir/" \;
  print_success "Debian package build finished"
}

build_binary() {
  local output_dir=$1
  local build_version=${2:-}
  print_step "Build Release Binary"
  mkdir -p "$output_dir"
  if [ -z "$build_version" ]; then
    build_version="$(git -C "$PROJECT_DIR" describe --tags --always --dirty 2>/dev/null || echo dev)"
  fi
  (
    cd "$PROJECT_DIR"
    run_cmd env GOCACHE="$GO_CACHE_DIR" GOMODCACHE="$GO_MOD_CACHE_DIR" go build -ldflags "-X main.Version=$build_version" -o "$output_dir/z13center" .
  )
  print_success "Release binary built"
}

create_tag() {
  print_step "Create Tag"
  (cd "$PROJECT_DIR" && run_cmd git tag -a "$RELEASE_VERSION" -m "Release $RELEASE_VERSION")
  (cd "$PROJECT_DIR" && run_cmd git push "$REPO_REMOTE" "$RELEASE_VERSION")
  print_success "Tag $RELEASE_VERSION published"
}

create_release() {
  local notes_file=$1
  local assets=("$RELEASE_DIR/z13center")

  shopt -s nullglob
  assets+=("$RELEASE_DIR"/*.pkg.tar.*)
  assets+=("$RELEASE_DIR"/*.tar.zst)
  assets+=("$RELEASE_DIR"/*.deb)
  shopt -u nullglob

  print_step "Create GitHub Release"
  (
    cd "$PROJECT_DIR"
    run_cmd gh release create "$RELEASE_VERSION" \
      --notes-file "$notes_file" \
      --title "$RELEASE_VERSION" \
      "${assets[@]}"
  )
  print_success "GitHub release created"
}

summarize_release() {
  print_step "Summary"
  echo -e "  ${BOLD}Version:${NC}      $RELEASE_VERSION"
  echo -e "  ${BOLD}Branch:${NC}       $REPO_BRANCH"
  echo -e "  ${BOLD}Artifacts:${NC}    $RELEASE_DIR"
  echo -e "  ${BOLD}Remote:${NC}       $REPO_REMOTE"
  echo -e "  ${BOLD}Latest commit:${NC} $(git -C "$PROJECT_DIR" rev-parse --short HEAD)"
  print_success "Release flow complete"
}

require_release_version() {
  if [ -z "${RELEASE_VERSION:-}" ]; then
    choose_bump
  fi
}

run_code_only_flow() {
  print_step "Code Only"
  commit_changes
  push_branch
  print_success "Code changes committed and pushed"
}

run_full_release() {
  local notes_file build_arch build_debian
  notes_file=$(mktemp)
  trap 'rm -f "$notes_file"' EXIT

  preflight
  commit_changes
  push_branch
  require_release_version
  print_info "Selected release version: $RELEASE_VERSION"
  prepare_release_notes "$notes_file"

  build_binary "$RELEASE_DIR" "$RELEASE_VERSION"

  if confirm "Build Arch package?"; then
    build_arch_package "$RELEASE_DIR"
  fi

  if confirm "Build Debian package?"; then
    build_debian_package "$RELEASE_DIR"
  fi

  create_tag
  create_release "$notes_file"
  summarize_release
}

pause_for_input() {
  echo ""
  read -r -p "Press Enter to continue..." _
}

run_menu() {
  local choice
  while true; do
    print_header
    ensure_repo
    ensure_branch
    print_menu_box
    echo ""
    read -r -p "Select action: " choice

    case "$choice" in
      1)
        print_step "Status"
        status_report
        pause_for_input
        ;;
      2)
        preflight
        pause_for_input
        ;;
      3)
        print_step "Commit Changes"
        commit_changes
        pause_for_input
        ;;
      4)
        push_branch
        pause_for_input
        ;;
      5)
        run_code_only_flow
        pause_for_input
        ;;
      6)
        print_step "Select Version"
        choose_bump
        print_success "Selected version $RELEASE_VERSION"
        pause_for_input
        ;;
      7)
        print_step "Build Binary"
        if confirm "Build binary with selected release version?"; then
          require_release_version
          build_binary "$RELEASE_DIR" "$RELEASE_VERSION"
        else
          build_binary "$RELEASE_DIR"
        fi
        pause_for_input
        ;;
      8)
        build_arch_package "$RELEASE_DIR"
        pause_for_input
        ;;
      9)
        build_debian_package "$RELEASE_DIR"
        pause_for_input
        ;;
      10)
        run_full_release
        pause_for_input
        ;;
      q|Q)
        exit 0
        ;;
      *)
        print_error "Invalid selection"
        pause_for_input
        ;;
    esac
  done
}

main() {
  run_menu
}

main "$@"
