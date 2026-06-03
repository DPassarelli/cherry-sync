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
  # - Pull direction (remote -> local): the local repo's ignore set still governs;
  #   a remote file that the local repo would ignore is not pulled.
