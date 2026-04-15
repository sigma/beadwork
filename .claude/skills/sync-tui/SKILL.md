---
name: sync-tui
description: Sync the tui branch with upstream origin, rebasing local changes and resolving conflicts
disable-model-invocation: true
allowed-tools: Bash Read Edit Grep Glob
---

Sync the `tui` jj bookmark with upstream `origin` and push to the `fork` remote.

Follow these steps in order:

## 1. Fetch upstream changes

```
jj git fetch --remote origin
```

If there are no new changes, report that and stop.

## 2. Rebase tui changes onto updated main

```
jj rebase -b tui -d main
```

## 3. Handle conflicts

If the rebase produces conflicts:
- Run `jj log` to identify which commits have conflicts
- For each conflicted commit:
  1. Create a child change with `jj new <change-id>` to work in
  2. Inspect the conflicted files (the parent's conflict markers will be visible)
  3. Read the files, understand the intent of both sides, and resolve them
  4. Run `jj squash` to fold the resolution back into the conflicted commit
  5. If the resolution goes wrong, `jj abandon` the child and start over
- After all conflicts are resolved, verify with `jj log` that no conflicts remain

## 4. Verify the result

- Run `go build ./...` to confirm the code compiles
- Run `go test ./...` to confirm tests pass
- If either fails, fix the issues in the appropriate commits

## 5. Offer to push

Once everything is clean and working, update the bookmark and ask the user if they want to push:

```
jj bookmark set tui -r <tip-of-tui-changes>
jj git push --remote fork --bookmark tui
```

Do NOT push without explicit user confirmation.
