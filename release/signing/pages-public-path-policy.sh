#!/usr/bin/env bash
# Shared rules for public GitHub Pages paths (sourced by other scripts).
pages_path_is_forbidden() {
  local rel="${1:-}"
  [[ -z "$rel" ]] && return 0
  case "$rel" in
    ..|../*|*/..|*/../*) return 0 ;;
    .git|.git/*|*/.git|*/.git/*) return 0 ;;
    cmd|cmd/*|internal|internal/*|docs|docs/*|.github|.github/*|.cursor|.cursor/*) return 0 ;;
    go.mod|go.sum|LICENSE|README.md|SECURITY.md|AGENTS.md) return 0 ;;
  esac
  return 1
}

pages_path_is_public() {
  local rel="${1:-}"
  pages_path_is_forbidden "$rel" && return 1
  case "$rel" in
    index.html|.nojekyll|pages-deploy-manifest.txt) return 0 ;;
    scripts/*) return 0 ;;
    lab/*) return 0 ;;
    stable/*) return 0 ;;
    *) return 1 ;;
  esac
}
