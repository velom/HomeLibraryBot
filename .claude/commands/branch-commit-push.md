Create a feature branch, commit all changes, and push to remote.

## Steps

1. Run `git -C $CWD status` to see current changes. If there are no changes, stop and inform the user.
2. Ask the user for a branch name if not provided as argument: `$ARGUMENTS`. If provided, use it directly.
3. Create and switch to the new branch: `git -C $CWD checkout -b <branch-name>`
4. Stage all relevant changes (avoid secrets like .env files). Use specific file names, not `git add -A`.
5. Run `git -C $CWD diff --cached` to review staged changes.
6. Compose a concise commit message based on the changes (1-2 sentences, focus on "why").
7. Commit with the message.
8. Push the branch to origin with upstream tracking: `git -C $CWD push -u origin <branch-name>`
9. Show the user the result: branch name, commit hash, and remote URL.
