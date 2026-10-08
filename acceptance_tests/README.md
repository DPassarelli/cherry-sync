# Feature map

The `.feature` files in this folder are csync's specification. This page puts them in the order a user meets them, so the main flows can be read before the details. A test fails if a feature is missing from this page.

## Sync with explicit paths

1. [Name the source and destination](name-source-and-destination.feature) on the command line.
2. [Report differences between local and remote](report-differences.feature), which are:
   - [ordered like a file tree](order-reported-actions.feature) and numbered for selection,
   - [explained](explain-differences.feature) by how the two copies differ,
   - free of [files git ignores](honor-gitignore.feature) and of [tooling metadata](withhold-tooling-metadata.feature) such as `.git`,
   - followed by a note of [what was held back](disclose-withheld-changes.feature).
3. [Select and sync](select-and-sync.feature) some or all of the changes, including deletions.
4. [Log the run](log-each-run.feature) to a file, and [prune old logs](prune-run-logs.feature).

## Sync with a saved remote

[Save the remote](sync-with-saved-remote.feature) in the project's `.csync.toml` and run `csync push` or `csync pull`. From there the flow is the same as steps 2–4 above.

## Set up a sync by answering prompts

[Run csync with no arguments](prompt-for-source-and-destination.feature) to be asked for the source and destination. This is specified but not yet implemented.

## When something goes wrong

- [Every path is treated as a literal path](guard-unsafe-input.feature), and paths csync can't handle safely are refused.
- [A transfer whose remote goes silent](bound-stalled-transfers.feature) ends with an explanation instead of hanging.

## Information about csync

- [Show help](show-help.feature)
- [Report the version](report-version.feature)
- [Report the license](report-license.feature)
