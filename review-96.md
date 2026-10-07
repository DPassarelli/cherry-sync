# Review of feature files against #96

Scope: 14 feature files, 135 scenarios (14 of them `@wip`). 51 scenarios exceed 5 steps. Every file already opens with an "In order to… / I want…" story.

Findings are grouped as cross-cutting themes first (the eventual work units), then per-file notes. Each finding has an ID for triage.

---

## Part 1: Cross-cutting themes

### T1. Plumbing `Given` that builds the remote (81 uses)

`that all of the files are identical between local and remote` is what creates the remote mirror in the harness (`steps_fixtures_test.go` errors with "missing 'identical between local and remote' step?" without it). It is then contradicted by the very next `Given` ("README.md has been changed locally"), exactly as #96 Example 2 describes.

- Fix: make a mirrored remote the default fixture whenever a local directory is set up; the step becomes a no-op (or is deleted from scenarios entirely). Keep `an empty remote directory` as an explicit, behavior-relevant override.
- Payoff: removes one line from 81 scenarios; the biggest single lever on the 5-line limit.
- Open question: a few scenarios need the remote to *not* exist (missing source, `~user` rejection). Need to confirm the default can be lazy or harmless there.

### T2. Redundant `the reported change count should be N` after an action table (21 uses)

Appears in compare-directories (5), honor-gitignore (10), select-and-sync (4), saved-targets (1), explain-differences (1). The table already implies the count, and the count step names an implementation concept.

- Fix: have `the reported actions should be:` also check the count in its code-behind (as #96 suggests), then delete the extra step. Where the count *is* the subject (explain-differences' last scenario), replace with a table.
- `no actions should be reported` + `change count should be 0` is the same pattern (compare-directories, select-and-sync).

### T3. Verbose transfer-outcome `Then`s in select-and-sync (and saved-targets, interactive-mode)

Scenarios assert `the reported sync count should be N` *plus* one `the file "X" should be identical / still differ / not exist` line per file (up to 5 `Then`/`And` lines). The sync count is implied by the per-file outcomes.

- Fix: a single table step, e.g.
  ```
  Then the remote should end up with:
    | path          | state      |
    | LICENSE       | synced     |
    | src/parser.go | unchanged  |
  ```
  and drop the sync count except in the one scenario that's *about* the summary ("reports both", removed count).

### T4. Guard assertions stacked as a second behavior (exit codes)

Many scenarios end `Then csync should return exit code 0 / a non-zero exit code` followed by the real assertion. log-each-run says so outright ("Exit code 0 is a guard rather than a second behavior") in several comments. That conflicts with "one scenario, one rule".

- Options: (a) move the guard into the code-behind of the primary step (e.g. a "run succeeded" precondition inside `csync should report where it logged the run`); (b) keep it but accept it as a documented exception. I lean (a) for success guards and keep explicit exit-code assertions only where exit status *is* the behavior (usage errors, rejections).

### T5. White-box "Teeth" comments

Most scenarios carry a long comment explaining the mutation that turns it red, often naming internals: `rsyncArgs`, `localSyncDir`, `gitignoreExcludes`, `isGitWorkTree`, `GitDirExcluded`, `parseActions`, `--checksum`, `-8`, filter-rule `--delete`, "the single deferred closure every exit shares". Some run 10–12 lines, longer than the scenario.

- The scenarios themselves are mostly black-box; it's the comments that make the files read as test specs rather than documentation.
- Options: (a) trim each comment to the one-sentence *why* a user would care, and move mutation/teeth notes to the step definitions or the PR; (b) keep teeth notes but strip identifier names. Needs a decision, since TESTING.md's "prove the test has teeth" rule is presumably why these exist.

### T6. Security and input-validation scenarios are scattered

| Scenario | Lives in |
|---|---|
| Source that looks like an rsync option (`-e`) | compare-directories |
| Configured remote that looks like an rsync option | saved-targets |
| Empty path argument | invoke-command |
| Empty `remote` value in `.csync.toml` | saved-targets |
| `~user` shortcut rejected (local) | invoke-command |
| `~user` shortcut rejected (saved remote) | saved-targets |
| Deletion candidate with a glob character not offered | select-and-sync |
| Filename containing a newline (`@wip`) | select-and-sync |

- Fix: a new "Guard against unsafe input" feature (per #96 Example 3). The `~user` and empty-remote ones are arguably validation rather than security; could be a sibling "Reject unusable paths" feature instead.

### T7. Exclusion/disclosure behavior is split across three features

`.git` exclusion and disclosure (honor-gitignore, 6 scenarios), `.csync.toml` exclusion and disclosure (saved-targets, 2 scenarios + 1 in honor-gitignore), withheld-change summaries (honor-gitignore, 4). None of the `.git` or `.csync.toml` ones are about `.gitignore`.

- Fix: split honor-gitignore into (a) "Leave ignored files out" (the `.gitignore`/exclude rules, push and pull), (b) "Never sync tooling metadata" (`.git`, submodule `.git`, `.csync.toml`), (c) "Disclose what was withheld" (withheld rows, summaries). Each could then have a real `Background`, which honor-gitignore currently can't (its header comment explains why).

### T8. Feature titles that name the mechanism, not what the user sees

- "Compare directories" → "Report differences between local and remote" (#96's own suggestion).
- "Invoke command" → e.g. "Name the source and destination".
- "Honor .gitignore when comparing" → see T7.
- Others read fine.

### T9. Stale header comments

- select-and-sync, saved-targets, interactive-mode headers still say "Scenarios are brought into the run one at a time…" and saved-targets lists "New steps this feature will need at drill-in" although those steps exist.
- interactive-mode's "HEADS UP" names an invoke-command scenario ("No arguments — show usage and exit non-zero") that has since been renamed.
- Not a BDD-principle issue strictly, but it undercuts "reads like documentation".

### T10. Tension with TESTING.md (needs a decision, not a fix)

- TESTING.md says "Imperative over declarative" (exact command, exact output). #96 asks for real-world detail *only where it bears on the behavior*. These mostly agree, but e.g. explain-differences' full literal detail string and report-license's three `contains` checks are imperative at the cost of brittleness.
- TESTING.md mandates TODO blocks at the bottom of feature files. #96's "read like documentation" goal doesn't forbid them, but they're long in several files. Leave as is unless you want to revisit.

---

## Part 2: Per-file notes

Only findings not already covered by a theme above.

### compare-directories (7 scenarios)
- C1. `-e` scenario → T6. 
- C2. "A comparison that fails reports what rsync said" is an error-reporting behavior. It fits the story loosely; could stay.
- C3. "One/Two of the files are different" titles describe fixtures, not rules. E.g. "A file changed locally is reported as an update", "A file added locally is reported as a create". "Two of the files" adds nothing over two single-file scenarios and could be dropped or retitled as "Each change is reported".
- C4. Coverage gap: no `delete` row here (it lives in select-and-sync). Reporting a deletion belongs with reporting.

### invoke-command (9)
- I1. Shortest, cleanest file. Failure paths well covered.
- I2. `~user` and empty-path rejections → T6. "~user rejected" also asserts "no run log" — a second behavior (already covered by log-each-run's "rejected before it reaches rsync").

### show-help (5), report-version (2), report-license (2)
- H1. Fine overall. report-license's first scenario has 3 `contains` lines (5 steps); could be one ("should contain the MIT license text") with the specifics in code-behind.
- H2. No failure paths, which is reasonable for these.

### order-reported-actions (2)
- O1. "Ordered like a file tree" tests five sort keys in one 11-row scenario with a 14-line comment. Split into one scenario per key (dot entries first, files before folders, numbers by value, numbers before letters, case-insensitive letters).
- O2. "Each reported change is labeled with its selection number" fits the select-and-sync story better than ordering.
- O3. The contract comment block at the top duplicates what split scenarios would say.

### explain-differences (6)
- E1. Good model of small, focused scenarios.
- E2. Last scenario asserts count, exit code, and detail (T2, T4). The detail is the behavior.

### bound-stalled-transfers (1)
- B1. Fine. The Background's identical-step is T1.
- B2. Only one scenario; no "comparison is not bounded on silence" counterpart, though the header explains why that's deliberate.

### select-and-sync (26, 2 `@wip`)
- S1. Biggest offender on length (14 over 5 steps), mostly T1 + T3.
- S2. Idempotence scenarios use two actions in the `When` (`When I run … And I run … a second time`). Violates "one action". Rephrase with a `Given` for the first run: `Given I have synced every change` / `When I run csync again` / `Then no actions should be reported`.
- S3. "Selecting 'a'" and "Accepting the default" are near-duplicates (fine as separate rules, just noting).
- S4. Deletion scenarios (detection vs. apply vs. decline vs. mixed) are well-separated. Detection belongs in compare-directories (C4).
- S5. Range/list grammar scenarios carry a repeated row-numbering comment (4×). If T3's table states the outcome per file, the comment is unnecessary.
- S6. Pull transfer is `@wip` here; only saved-targets' pull covers pull end to end.

### interactive-mode (4, all `@wip`)
- N1. Entire feature is `@wip`. Review deferred until it's drilled in, but noting: the four-line answer docstring repeats in every scenario; "round-trips into a later push" puts a full csync run in a `Given` (fine as past tense, but it's 7 steps).
- N2. Stale HEADS UP (T9).

### saved-targets (15)
- G1. Each scenario repeats a 4–6 line fixture (local files + `.csync.toml` docstring). A `Background` with a valid `.csync.toml` would trim most; the invalid-config scenarios override it.
- G2. `.csync.toml` exclusion/disclosure → T7. `-e`, empty remote, `~user` → T6.
- G3. "~ normalized" and "trailing slash normalized" are operand-normalization rules that also apply to argv operands (invoke-command has a `~` version). Possibly one "Normalize operands" feature.
- G4. "push/pull rejects extra arguments" are usage errors; could sit with invoke-command's usage errors.

### honor-gitignore (22)
- R1. Biggest offender (17 over 5 steps, max 10). T1 + T2 + T7 + long fixtures.
- R2. Fixture compression: `Given a local git repository ignoring "*.log", containing:` would fold the `.gitignore` docstring into one step for the common case.
- R3. "An ignored file is left out" and "names the ignored change it withheld" share an identical 8-step setup. After T7 they'd share a Background.
- R4. "Non-repository local side excludes nothing" asserts 4 things (actions, count, no withheld, no .git disclosure). Split or reduce to the actions table.
- R5. "Run log records every withheld file" → log-each-run.
- R6. "A gitignored .csync.toml is disclosed once" asserts via the run log; the user-visible behavior is the disclosure. Worth checking whether a screen-level assertion is possible.
- R7. Titles prefixed "Pull direction —" are good; push equivalents have no prefix. Consistent.

### log-each-run (33)
- L1. Largest file. Story is fine but it covers four distinct concerns: where the log goes (location, privacy, disclosure), what it records (version, commands, operands, classifications, selections, exclusions), resilience (can't-write warnings), and when no log is written. Split into 3–4 features.
- L2. `I have started csync but not yet answered the prompt` / `When I look for the log file` exposes harness mechanics (suspending at the prompt to read mid-run). "Look for the log file" isn't a user action. Consider `When csync is waiting for my selection` / `Then the log should already record …`.
- L3. Several scenarios exist mainly to pin internals: "duration logged as whole milliseconds", "path containing a double quote… without forging a boundary", "reports the log it has been writing all along". Defensible (they're observable in the log file), but flag for a "need to have?" check per #96.
- L4. The "cannot write its log" block repeats a 4-line setup 4×; a nested feature with its own Background would cut it to 2 lines each.

### prune-run-logs (6)
- P1. Good model: 3–4 steps each, one behavior each. Only T1 applies.

---

## Part 3: Can a reader see the major user flows?

Partially. The main flow (compare → review → select → transfer → log) is spread across compare-directories, order-reported-actions, explain-differences, select-and-sync and log-each-run. Nothing ties them together, and the files list alphabetically, which doesn't match the flow.

Options:
1. A short "user journey" section in README (or a `features/README.md`) listing the flows in order and pointing at the feature for each step.
2. Number or prefix feature filenames by flow stage.
3. A single high-level "happy path" feature (push and pull end to end, 3–4 steps each) that acts as the table of contents.

These aren't mutually exclusive; (1) is the cheapest.
