# Scenarios are brought into the run one at a time as we implement them. Each
# not-yet-implemented scenario carries its own @wip tag (excluded via the
# "~@wip" filter in features_test.go); drop a scenario's tag when we drill in.
#
# HEADS UP — conflict to resolve at drill-in: invoke-command.feature currently
# asserts "No arguments — show usage and exit non-zero". This feature redefines
# no-args as interactive prompting, so that scenario must be retired when the
# first scenario here goes live. Don't ship both.
#
# @remote runs every scenario over a fake SSH remote, so the prompted sync
# actually transfers. The remote operand now arrives from a prompt answer rather
# than argv; the @remote harness must rewrite it the same way.
#
# New steps this feature will need at drill-in:
#   - I run "csync" and respond with: <docstring of newline-separated answers,
#     fed to stdin in prompt order>
#   - the ".csync.toml" in the project directory should not exist
#   - the ".csync.toml" in the project directory should contain: <docstring>
@remote
Feature: Interactive mode

  In order to start a sync without remembering the argument order, I want csync
  run with no arguments to prompt me for the source and destination — and
  optionally remember the remote for next time.

  @wip
  Scenario: No arguments — prompting drives a sync
    # The answers are fed on stdin in prompt order: source, then destination,
    # then the change selection, then the save y/N. Here: source "./project",
    # destination "user@host:/project", "a" to sync all, "n" to skip saving.
    # README is changed locally; after the prompted run it must match the
    # remote, proving the prompted operands flowed into the same compare/sync
    # path as the explicit two-arg form.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    When  I run "csync" and respond with:
      """
      ./project
      user@host:/project
      a
      n
      """
    Then  the reported sync count should be 1
    And   the file "README.md" should be identical between local and remote

  @wip
  Scenario: Declining the save prompt writes no config
    # Answering "n" to the final prompt must leave the project directory without
    # a .csync.toml. Guards against the tool writing config the user declined.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    When  I run "csync" and respond with:
      """
      ./project
      user@host:/project
      a
      n
      """
    Then  the ".csync.toml" in the project directory should not exist

  @wip
  Scenario: Accepting the save prompt writes the entered remote
    # Answering "y" must persist the REMOTE operand (the one carrying user@host:)
    # as `remote` in ./project/.csync.toml — not the local operand. The written
    # value is what `csync push`/`pull` will later read back.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    When  I run "csync" and respond with:
      """
      ./project
      user@host:/project
      a
      y
      """
    Then  the ".csync.toml" in the project directory should contain:
      """
      remote = "user@host:/project"
      """

  @wip
  Scenario: A saved target round-trips into a later push
    # Integration proof that the saved file is not just present but correct:
    # after interactive mode writes the config, `csync push` from the same
    # directory reuses it with no source/destination prompts and reconciles a
    # fresh change. If the saved `remote` were wrong or unparseable, this push
    # would fail or prompt — red.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   I run "csync" and respond with:
      """
      ./project
      user@host:/project
      n
      y
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory and respond with "a"
    Then  the reported sync count should be 1
    And   the file "README.md" should be identical between local and remote

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Empty answer at a prompt: decide reject-and-exit vs re-prompt. clig.dev
  #   favors re-prompting on a typo; v1 may just reject (non-zero) for
  #   simplicity. This choice is open — settle it before drafting.
  #
  # - Saving when .csync.toml already exists: overwrite, confirm-overwrite, or
  #   refuse? Don't clobber a hand-edited config silently.
  #
  # - No remote operand entered (both answers local, or both remote): there is
  #   nothing to save — skip the save offer. Ties into the "both local / both
  #   remote" decisions tracked in invoke-command.feature.
  #
  # - EOF / ctrl-c at a prompt (closed stdin before all answers): csync must
  #   abort cleanly, not transfer or write a partial config.
  #
  # - TTY gating (DECIDED — implementation deferred): interactive mode requires an
  #   interactive terminal. csync prompts only when stdin is a TTY; with piped or
  #   redirected stdin (CI, scripts) it does NOT prompt — bare `csync` and a
  #   missing-config push/pull both fall to a non-interactive error there. The
  #   non-TTY branch is already covered (the harness pipes stdin): see
  #   "A missing .csync.toml fails loudly" in saved-targets.feature. Testing the
  #   TTY branch needs a pseudo-terminal on stdin only (stdout/stderr stay
  #   separate buffers so the output parser still works). That PTY harness — and
  #   the choice between a real PTY and a forced-interactive escape hatch — is
  #   parked: a proven PTY testing methodology from separate TUI work is to be
  #   brought in once this feature ships and is released.
  #
  # - Offer setup on a missing config (TTY only): `csync push`/`pull` with no
  #   .csync.toml, when stdin is a TTY, drops into the prompts above and offers to
  #   save a target instead of erroring — the most reasonable expectation at a
  #   terminal. It's the bare-`csync` flow triggered by the saved-target verbs.
  #   Blocked on the PTY harness above; the non-TTY error counterpart already
  #   ships (saved-targets.feature).
