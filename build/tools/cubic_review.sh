#!/usr/bin/env bash
set -euo pipefail

REPO="Nuteres/depersonalization-service"
PR_NUM="${1:-}"

if [ -z "$PR_NUM" ]; then
  # Auto-detect PR for current git branch
  BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")
  if [ -n "$BRANCH" ] && [ "$BRANCH" != "main" ]; then
    PR_NUM=$(gh pr view "$BRANCH" --json number -q .number 2>/dev/null || echo "")
  fi
fi

if [ -z "$PR_NUM" ]; then
  echo "Usage: $0 <pr-number>"
  echo "Or run inside a branch associated with an open pull request."
  exit 1
fi

echo "==> Проверка ревью Cubic AI для PR #$PR_NUM ($REPO)..."

# Fetch review summary from reviews endpoint
REVIEWS=$(gh api "repos/$REPO/pulls/$PR_NUM/reviews" --jq '.[] | select(.user.login == "cubic-dev-ai[bot]") | .body' 2>/dev/null || echo "")
if [ -n "$REVIEWS" ]; then
  echo "--- Общая сводка ревью Cubic ---"
  echo "$REVIEWS"
  echo "---------------------------------"
fi

# Fetch inline comments
COMMENTS=$(gh api "repos/$REPO/pulls/$PR_NUM/comments" --jq '.[] | select(.user.login == "cubic-dev-ai[bot]") | "📍 \(.path):\(.line // "общий")\n\(.body)\n"' 2>/dev/null || echo "")
if [ -n "$COMMENTS" ]; then
  echo "--- Замечания по коду ---"
  echo "$COMMENTS"
else
  if [ -z "$REVIEWS" ]; then
    echo "Cubic AI пока не оставил ревью для PR #$PR_NUM. Попробуйте через 30-60 секунд."
  else
    echo "Замечаний по отдельным строкам не обнаружено."
  fi
fi
