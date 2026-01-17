#!/bin/bash
# Trojan Release Build Script
# Usage: ./release.sh [version] [--dry-run]

set -e

REPO="shafishcn/trojan"
RELEASE_DIR="releases"
DRY_RUN=false
VERSION=""
TAG_NAME=""

while [[ $# > 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [version] [--dry-run]"
            echo "  version     Release version (e.g., v1.0.0), auto-generated if not provided"
            echo "  --dry-run   Show what would be done without actually doing it"
            exit 0
            ;;
        -*)
            echo "Unknown option: $1"
            exit 1
            ;;
        *)
            VERSION="$1"
            shift
            ;;
    esac
done

echoStep() {
    echo "[STEP] $1"
}

echoSuccess() {
    echo "[SUCCESS] $1"
}

echoWarn() {
    echo "[WARN] $1"
}

echoError() {
    echo "[ERROR] $1"
}

checkPrerequisites() {
    echoStep "Checking prerequisites..."

    if ! command -v git &> /dev/null; then
        echoError "git is not installed"
        exit 1
    fi

    if ! command -v go &> /dev/null; then
        echoError "go is not installed"
        exit 1
    fi

    if [ "$DRY_RUN" = false ] && ! command -v gh &> /dev/null; then
        echoError "GitHub CLI (gh) is not installed"
        echo "Install it from: https://cli.github.com/"
        exit 1
    fi

    echoSuccess "All prerequisites met"
}

getVersion() {
    if [ -n "$VERSION" ]; then
        TAG_NAME="$VERSION"
    else
        TAG_NAME=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
        if [ -z "$TAG_NAME" ]; then
            TAG_NAME="v$(date +%Y.%m.%d)-$(git rev-parse --short HEAD)"
        fi
    fi

    VERSION="${TAG_NAME#v}"

    echoStep "Version: $VERSION"
    echoStep "Tag: $TAG_NAME"
}

checkGitStatus() {
    echoStep "Checking git status..."

    if [ -n "$(git status --porcelain)" ]; then
        echoWarn "You have uncommitted changes:"
        git status --short
        echo ""
        read -p "Continue anyway? [y/N] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi

    if git tag --list | grep -q "^${TAG_NAME}$"; then
        echoWarn "Tag $TAG_NAME already exists!"
        read -p "Delete and recreate? [y/N] " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            git tag -d "$TAG_NAME"
            if [ "$DRY_RUN" = false ]; then
                git push origin :refs/tags/"$TAG_NAME" 2>/dev/null || true
            fi
        else
            echo "Aborting..."
            exit 1
        fi
    fi

    echoSuccess "Git status OK"
}

setupReleaseDir() {
    echoStep "Setting up release directory..."

    rm -rf "$RELEASE_DIR"
    mkdir -p "$RELEASE_DIR"

    echoSuccess "Created $RELEASE_DIR"
}

buildAllPlatforms() {
    echoStep "Building binaries for all platforms..."

    local build_date=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    local version="$VERSION"

    local platforms=(
        "linux:amd64:trojan-linux-amd64"
        "linux:arm64:trojan-linux-arm64"
        "darwin:amd64:trojan-darwin-amd64"
        "darwin:arm64:trojan-darwin-arm64"
    )

    for platform in "${platforms[@]}"; do
        IFS=':' read -r os arch filename <<< "$platform"
        echo "  Building $os/$arch..."

        if [ "$DRY_RUN" = true ]; then
            echo "    Would run: GOOS=$os GOARCH=$arch go build -o $RELEASE_DIR/$filename ."
        else
            if [ "$os" == "linux" ]; then
                GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -o "$RELEASE_DIR/$filename" .
            else
                GOOS=$os GOARCH=$arch go build -o "$RELEASE_DIR/$filename" .
            fi
        fi
    done

    if [ "$DRY_RUN" = false ]; then
        cp install.sh "$RELEASE_DIR/"
    fi

    echoSuccess "Built ${#platforms[@]} binaries"
}

createChecksums() {
    echoStep "Creating checksums..."

    if [ "$DRY_RUN" = true ]; then
        echo "  Would create SHA256SUMS"
    else
        cd "$RELEASE_DIR"
        sha256sum * > SHA256SUMS
        cd ..
        echoSuccess "Created SHA256SUMS"
    fi
}

showSummary() {
    echo ""
    echo "========================================"
    echo "       Release Summary"
    echo "========================================"
    echo ""
    echo "  Version:   $VERSION"
    echo "  Tag:       $TAG_NAME"
    echo "  Files:"
    ls -lh "$RELEASE_DIR" 2>/dev/null | tail -n +4 | while read -r line; do
        echo "    $line"
    done
    echo ""
    echo "  Repo:      https://github.com/$REPO"
    echo "  Release:   https://github.com/$REPO/releases/tag/$TAG_NAME"
    echo ""
}

createRelease() {
    echoStep "Creating GitHub release..."

    if [ "$DRY_RUN" = true ]; then
        echo "  Would create release with tag: $TAG_NAME"
        echo "  Would upload binaries"
        return
    fi

    git tag -a "$TAG_NAME" -m "Release $VERSION"

    local notes="Release $VERSION

Changes: $(git log --oneline --decorate -10 | head -10)

Downloads: trojan-linux-amd64, trojan-linux-arm64, trojan-darwin-amd64, trojan-darwin-arm64

SHA256SUMS:
$(cat "$RELEASE_DIR/SHA256SUMS")"

    git push origin "$TAG_NAME"

    gh release create "$TAG_NAME" \
        --title "Trojan $VERSION" \
        --notes "$notes" \
        "$RELEASE_DIR"/*

    echoSuccess "Release created: https://github.com/$REPO/releases/tag/$TAG_NAME"
}

main() {
    echo ""
    echo "Trojan Release Build Script"
    echo ""

    checkPrerequisites
    getVersion
    checkGitStatus
    setupReleaseDir
    buildAllPlatforms
    createChecksums
    showSummary

    if [ "$DRY_RUN" = true ]; then
        echo "[DRY RUN] No changes were made"
        echo ""
        echo "To actually create the release, run without --dry-run"
    else
        read -p "Create GitHub release? [y/N] " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            createRelease
        else
            echo "Skipped release creation"
            echo "To create release later, run:"
            echo "  gh release create $TAG_NAME --title Trojan-$VERSION releases/*"
        fi
    fi
}

main
