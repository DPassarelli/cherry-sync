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
  # the local operand and uses that ignore set for push and pull alike. These
  # scenarios set up the *local* directory as a git work tree; git is run only
  # against that side.

  Background:
    Given a local git repository containing these files:
      """
      src/main.go
      README.md
      """
    And   the repository's ".gitignore" contains:
      """
      *.log
      """

  Scenario: An ignored file is left out of the comparison
    # Teeth: README.md (tracked, changed) MUST report; debug.log (ignored, newly
    # added) MUST NOT. Remove the exclusion and debug.log surfaces as a `create`
    # and the count becomes 2 — red. Break compare entirely and the README.md row
    # vanishes — also red. So this fails for the right reason and proves the
    # exclusion is *targeted*, not a blanket "report nothing".
    Given that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    And   that the file "debug.log" has been added locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
    And   the reported change count should be 1

  # ---------------------------------------------------------------------------
  # TODO: sibling scenarios, each its own behavior — drafted as we drill in.
  # ---------------------------------------------------------------------------
  #
  # - Disclosure: compare announces the count of excluded paths
  #   (e.g. "excluding N gitignored paths"). The user's only signal that files
  #   were hidden, since there's no opt-out flag.
  #
  # - Local side is NOT a git repository: nothing is excluded — the feature is a
  #   pure no-op and every differing file reports as before.
  #
  # - Ignored via .git/info/exclude (no matching .gitignore line): still excluded.
  #   Guards that the trigger is "is a git work tree?" + --exclude-standard, not
  #   the literal presence of a .gitignore file.
  #
  # - Syncing a subdirectory of the repo: ignored paths still resolve correctly.
  #   Guards the recipe — run `git ls-files` in the sync dir and anchor each
  #   emitted path with a leading "/", so a top-level ignore can't float onto a
  #   same-named nested path.
  #
  # - Pull direction (remote -> local): the local repo's ignore set still governs;
  #   a remote file that the local repo would ignore is not pulled.
