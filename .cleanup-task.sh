#\!/bin/bash
# Auto-generated cleanup script for this task worktree

TASK_NUMBER="11"
TASK_ID="$(printf "%04d" "$TASK_NUMBER")"
BRANCH_NAME="feature/task-${TASK_ID}"
WORKTREE_DIR="$(pwd)"
PARENT_REPO="$(dirname "$WORKTREE_DIR")"

echo "🧹 Cleaning up task $TASK_NUMBER..."

# Go to parent repo
cd "$PARENT_REPO"

# Remove worktree
echo "🗑️ Removing worktree: $WORKTREE_DIR"
git worktree remove "$WORKTREE_DIR" --force 2>/dev/null || {
    echo "⚠️ Could not remove worktree automatically. Run manually:"
    echo "   git worktree remove '$WORKTREE_DIR' --force"
}

# Delete branch
echo "🌿 Deleting branch: $BRANCH_NAME"
git branch -D "$BRANCH_NAME" 2>/dev/null || {
    echo "⚠️ Could not delete branch automatically. Run manually:"
    echo "   git branch -D '$BRANCH_NAME'"
}

echo "✅ Cleanup complete for task $TASK_NUMBER"
