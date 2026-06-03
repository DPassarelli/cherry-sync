# @git scenarios need `git` on the test host (the local operand is set up as a
# real work tree). The runner skips them when git is absent — see features_test.go.
#
# Each not-yet-implemented scenario carries @wip (excluded via the "~@wip" filter
# in features_test.go); drop a scenario's tag when we drill in.
@git
Feature: Honor .gitignore when comparing

  In order to keep build artifacts and other ignored files out of a sync,
  I want files the local git repository ignores left out of the comparison,
  so the diff shows only files I'd actually consider moving.

  # The local side governs in both directions: csync runs `git ls-files` against
  # the local operand and uses that ignore set for push and pull alike. Each
  # scenario sets up its own local side — most as a git work tree, the no-op case
  # as a plain directory — because the setups diverge enough that a shared
  # Background would mask the very thing some scenarios test (e.g. a Background
  # .gitignore would hide whether .git/info/exclude is honored).

  Scenario: An ignored file is left out of the comparison
    # Teeth: README.md (tracked, changed) MUST report; debug.log (ignored, newly
    # added) MUST NOT. Remove the exclusion and debug.log surfaces as a `create`
    # and the count becomes 2 — red. Break compare entirely and the README.md row
    # vanishes — also red. So this fails for the right reason and proves the
    # exclusion is *targeted*, not a blanket "report nothing".
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """
    And   that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
    And   the reported change count should be 1

  Scenario: The comparison discloses how many ignored paths it hid
    # Teeth: hiding debug.log is silent unless csync says so, and disclosure is
    # the user's only signal since there's no opt-out flag. The count is 1 — just
    # debug.log — even though the action list (README.md only) is identical to the
    # scenario above. Drop the disclosure line and this goes red while the actions
    # stay green, proving the count is reported in its own right.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """
    And   that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported excluded count should be 1

  Scenario: A non-repository local side excludes nothing
    # Teeth: this local directory is NOT a git work tree, yet it carries a
    # .gitignore naming *.log. Because the trigger is "is a git work tree?" — not
    # "does a .gitignore exist?" — nothing is excluded: debug.log surfaces as a
    # create alongside the README.md update, and no disclosure line is printed.
    # Drop the work-tree guard and `git ls-files` runs against a non-repo, which
    # errors — the comparison fails outright and reports no actions at all (red).
    # The guard is what keeps a non-repo a clean no-op. Runs only where git is
    # installed (@git), so a pass proves the gate is work-tree membership, not the
    # git binary simply being absent.
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """
    And   the directory's ".gitignore" contains:
      """
      *.log
      """
    And   that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
      | create | debug.log |
    And   the reported change count should be 2
    And   no gitignored paths should be reported as excluded
    And   the .git directory should not be reported as excluded

  Scenario: A file ignored via .git/info/exclude is left out
    # Teeth: this repository has NO .gitignore at all — the ignore rule lives only
    # in .git/info/exclude (git's per-clone, uncommitted ignore list). debug.log
    # must still be excluded, because csync drives `git ls-files --exclude-standard`,
    # which honors .gitignore, .git/info/exclude, and global excludes alike. Key the
    # exclusion on the literal presence of a .gitignore file and this repo has none,
    # so debug.log would surface and the count become 2 — red. Proves the trigger is
    # "is a git work tree?" + --exclude-standard, not "is there a .gitignore?".
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".git/info/exclude" contains:
      """
      *.log
      """
    And   that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
    And   the reported change count should be 1

  Scenario: A top-level ignore does not float onto a same-named nested path
    # Teeth — the leading-"/" anchoring in gitignoreExcludes. The repo ignores the
    # top-level build/ (via "/build/", anchored to the repo root by git), but a
    # *different* src/build/ is NOT ignored. Both hold a changed file. Anchoring
    # each emitted exclude with a leading "/" pins it to the transfer root, so the
    # exclude hits the real top-level build/artifact.o (absent) yet leaves
    # src/build/keep.go (present). Drop the "/" prefix and rsync reads "build/" as
    # a floating basename match at any depth — it swallows src/build/ too and
    # keep.go vanishes (red). Break exclusion entirely and build/artifact.o
    # reappears as a second change (also red). Verified against GNU rsync 3.4.1 and
    # openrsync v29: both float an unanchored "build/" and both honor "/build/".
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      src/build/keep.go
      build/artifact.o
      """
    And   the repository's ".gitignore" contains:
      """
      /build/
      """
    And   that all of the files are identical between local and remote
    And   that the file "build/artifact.o" has been changed locally
    And   that the file "src/build/keep.go" has been changed locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path              |
      | update | src/build/keep.go |
    And   the reported change count should be 1

  Scenario: The local .git directory is never offered for sync
    # Teeth — the explicit "/.git/" exclude. git never reports its own .git/ as
    # ignored (it special-cases that directory), so without an explicit exclude a
    # push from a repo offers every .git/ object for transfer — pure noise, and it
    # would clobber the other side's git state (HEAD, index, refs, hooks). Pushing
    # a fresh repo (real .git/ from git init) to an empty remote, only the working
    # files are offered; the whole .git/ tree is gone. Drop the "/.git/" pattern and
    # dozens of .git/ creates flood the list (red). Verified on GNU rsync 3.4.1 that
    # an anchored "/.git/" exclude zeroes them out.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path        |
      | create | README.md   |
      | create | src/main.go |
    And   the reported change count should be 2

  Scenario: The .git exclusion is disclosed even when nothing is gitignored
    # The .git/ exclusion is invisible by mechanism — git never lists it — so csync
    # must announce it, and must do so even when there's no .gitignore at all (zero
    # gitignored paths). This repo has no .gitignore; the disclosure still reports
    # the .git directory, with no gitignored-path count. Drop the disclosure, or
    # gate it on a gitignored count > 0, and this goes red.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the .git directory should be reported as excluded
    And   no gitignored paths should be reported as excluded

  @remote
  Scenario: Pull direction — the local repo's ignore set still governs
    # "Local repo governs both directions": on a pull (remote -> local) csync still
    # derives the ignore set from the LOCAL side — here the destination — because
    # localSyncDir picks the non-remote operand. The remote-only notes.txt is
    # pulled (create); debug.log, which differs and would otherwise be received, is
    # held back because the local repo ignores *.log. @remote keeps the source's
    # host: spec so csync sees it as remote and localSyncDir resolves to the local
    # destination. Teeth: make localSyncDir always pick the source and git runs
    # against the remote spec (not a local dir) -> no exclusion -> debug.log pulled
    # and the count becomes 2 (red); breaking exclusion entirely does the same.
    # Verified the pull exclude behavior by experiment (GNU rsync 3.4.1).
    #
    # Scope: covers a file present on BOTH sides (ignored locally). A remote-ONLY
    # ignored file can't be excluded this way — see the TODO below.
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      debug.log
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """
    And   that all of the files are identical between local and remote
    And   that the file "debug.log" has been changed locally
    And   that the file "notes.txt" has been added on the remote
    When  I run "csync user@host:/project ./project"
    Then  the reported actions should be:
      | action | path      |
      | create | notes.txt |
    And   the reported change count should be 1

  # ---------------------------------------------------------------------------
  # TODO: sibling scenarios, each its own behavior — drafted as we drill in.
  # ---------------------------------------------------------------------------
  #
  # - Syncing a *subdirectory* of the repo (csync ./repo/sub host:/dst): the
  #   emitted ignore paths must resolve relative to the sync root, not the repo
  #   root — `git ls-files` is run in the sync dir for exactly this reason. The
  #   anchoring half is covered above; this is the run-git-in-the-sync-dir half,
  #   still untested. Needs harness support for a sync operand below the repo root
  #   (today the runCsync placeholder maps only the bare "./project" token).
  #
  # - Floating ".git" exclude: today only the top-level "/.git/" is excluded
  #   (anchored, consistent with the gitignore-path anchoring). A repo with
  #   submodules carries nested .git dirs/files that an anchored exclude misses.
  #   Decide whether to add a floating ".git" exclude (matches at any depth) for the
  #   submodule case. Captured 2026-06-03.
  #
  # - Remote-ONLY ignored file on a pull: a file the local repo would ignore but
  #   that does NOT yet exist locally can't be excluded — `git ls-files` lists only
  #   files present in the local tree, so today such a file WOULD be pulled (it'll
  #   be gitignored locally once it lands, so it's low-harm, but it's still noise in
  #   the diff). Decide: accept it, or additionally filter the remote listing
  #   through the local ignore rules. Captured 2026-06-03.
