# Scenarios are brought into the run one at a time as we implement them. Each
# not-yet-implemented scenario carries its own @wip tag (excluded via the
# "~@wip" filter in features_test.go); drop a scenario's tag when we drill in.
#
# @remote runs every scenario over a fake SSH remote, so the push/pull scenarios
# transfer in real sender/receiver mode (see select-and-sync.feature for why
# local-to-local would hide direction bugs). The remote operand now also arrives
# from ./.csync.toml rather than argv, so the @remote harness must rewrite the
# configured remote the same way it rewrites an argv operand.
#
# New steps this feature will need at drill-in:
#   - a ".csync.toml" in the project directory containing: <docstring>
#   - I run "csync push|pull" from the project directory  (chdir so cwd-only
#     discovery finds the dotfile)
#   - the reported error should mention "<text>"
@remote
Feature: Saved sync targets

  In order to avoid retyping a remote I sync with often, I want to save it once
  in the project's .csync.toml and refer to it with `csync push` / `csync pull`.

  Scenario: Push resolves the saved remote as the destination
    # Teeth: push must read `remote` from ./.csync.toml and use it as the
    # destination, with the project directory (cwd) as the source. README is
    # changed locally; after `csync push` it must match the remote. Point the
    # config at the wrong side (or ignore it) and README stays divergent — red.
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory and respond with "a"
    Then  the reported sync count should be 1
    And   the file "README.md" should be identical between local and remote

  Scenario: Pull resolves the saved remote as the source
    # The mirror of push: pull must use the configured `remote` as the SOURCE and
    # the project directory as the destination. A file that exists only on the
    # remote is brought down. Swap source/destination resolution and this file
    # never lands locally — red.
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    And   that the file "notes.txt" has been added on the remote
    When  I run "csync pull" from the project directory and respond with "a"
    Then  the reported sync count should be 1
    And   the file "notes.txt" should be identical between local and remote

  Scenario: Push reports the resolved source and destination
    # Resolution proof, transfer-free: csync echoes the operands it resolved
    # BEFORE running rsync — the same pre-compare echo the invoke-command display
    # scenarios read — so this asserts only those two lines, not a transfer.
    # `push` must read `remote` from ./.csync.toml and report it as the
    # destination, with the project directory itself (".") as the source. Drop
    # the config read (push unhandled) and no Source/Destination pair is echoed —
    # red. (The "." display is a v1 choice; see TODO below.)
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    When  I run "csync push" from the project directory
    Then  the reported source should be "."
    And   the reported destination should be "user@host:/project"

  Scenario: csync does not offer its own .csync.toml as a change
    # csync's config file is local tooling metadata, like .git/ — never a
    # syncable file. It's exercised in explicit mode (no push) to prove the
    # exclusion is unconditional, not tied to the push/pull verbs: with a
    # .csync.toml present and README changed, README must be the ONLY reported
    # change. Drop the exclusion and .csync.toml surfaces as a second `create` —
    # the count becomes 2 and the action list grows a row, red.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "n"
    Then  the reported actions should be:
      | action | path      |
      | update | README.md |
    And   the reported change count should be 1

  Scenario: csync discloses that it held back its .csync.toml
    # Transparency, like the .git disclosure: when csync silently withholds a
    # file with no opt-out, it must say so. With a .csync.toml present and nothing
    # else to sync, the exclusion disclosure must name it. Drop the disclosure and the
    # user has no signal the config file was held back — red.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    When  I run "csync ./project user@host:/project"
    Then  the .csync.toml file should be reported as excluded

  Scenario: A missing .csync.toml fails loudly and transfers nothing
    # No silent fallback to a default. With no dotfile in the project directory,
    # `csync push` must error and exit non-zero BEFORE contacting rsync — the
    # changed README must still differ, proving no transfer was attempted.
    #
    # This is the NON-interactive branch: the harness pipes stdin, so no terminal
    # is present. When stdin IS a terminal, push/pull will instead offer to set up
    # a config (future work — see the TTY notes in interactive-mode.feature). That
    # branch does not contradict this one; they split on whether stdin is a TTY.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention ".csync.toml"
    And   the file "README.md" should still differ between local and remote

  Scenario: A .csync.toml with no remote key is rejected
    # The file exists but defines nothing usable. Rejected like a missing file:
    # non-zero exit, no transfer. Distinguishes "file present" from "remote
    # present" — an empty config is not an implicit default. The error must name
    # the file: drop the check and csync hands an empty operand to rsync, whose
    # failure names something else, not .csync.toml — red.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      # no remote defined yet
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention ".csync.toml"
    And   the file "README.md" should still differ between local and remote

  Scenario: A .csync.toml with an empty remote value is rejected
    # SECURITY.md: a config-sourced path gets the SAME validation as a CLI path.
    # An empty `remote` becomes the same footgun an empty argv path would (an
    # operand of ""), so it is rejected, not passed to rsync. This is the guard
    # that config is not a back door around path validation. The error naming
    # .csync.toml proves csync rejected it itself rather than letting rsync run on
    # an empty operand — remove the check and that proof goes red.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = ""
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention ".csync.toml"
    And   the file "README.md" should still differ between local and remote

  Scenario: Malformed TOML is rejected with a clear error
    # A syntactically broken dotfile must fail as malformed, not be silently
    # ignored or half-read. The "invalid .csync.toml" wording is what gives this
    # teeth independent of the empty-remote check: the TOML parser leaves the
    # remote empty on any parse error, so swallowing that error would fall through
    # to the "no remote" rejection — a different message. Asserting "invalid"
    # proves the parse failure itself is surfaced. Non-zero exit, no transfer.
    Given a local directory containing these files:
      """
      README.md
      """
    And   that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project
      """
    And   that the file "README.md" has been changed locally
    When  I run "csync push" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention "invalid .csync.toml"
    And   the file "README.md" should still differ between local and remote

  Scenario: push rejects extra arguments
    # The saved-target verbs take no operands — push resolves its target from
    # .csync.toml — so `csync push ./x` is a mistake, not an explicit sync of a
    # directory named "push". Without the guard, argv ["push", "./x"] is read as
    # an explicit source/destination pair and csync tries to sync a "push"
    # directory (exit 1 from rsync); the guard makes it the usage error (exit 2)
    # asserted here.
    When  I run "csync push ./project"
    Then  csync should return exit code 2
    And   the reported usage should begin with "usage: csync"

  Scenario: pull rejects extra arguments
    # The pull mirror of the guard above, so the rejection is not push-only.
    When  I run "csync pull ./project"
    Then  csync should return exit code 2
    And   the reported usage should begin with "usage: csync"

  Scenario: A configured remote that looks like an rsync option is treated as a path
    # Security regression guard — the config mirror of the same scenario in
    # compare-directories.feature, proving the config path source is not a bypass.
    # A `remote` of "-e" is rsync's remote-shell flag (--rsh = remote command
    # execution) if parsed as an option. config.Load does not reject it (the fix
    # is to neutralize, not blocklist); on pull it becomes the SOURCE operand, and
    # rsyncArgs's `--` makes rsync read it as a (nonexistent) path, so rsync errors
    # and csync exits non-zero. Delete the `--` and rsync would honor `-e` and exit
    # 0 — flipping this red, which also proves the non-zero exit is the guard at
    # work, not a config rejection. Pull (not push): an option-looking destination
    # would be a creatable path rsync's dry-run accepts. See SECURITY.md.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "-e"
      """
    When  I run "csync pull" from the project directory
    Then  csync should return a non-zero exit code

  Scenario: A saved remote with a ~ home shortcut is normalized before use
    # Teeth for #50: a remote written with a ~ home shortcut (host:~/project) is
    # taken literally by modern rsync, which resolves it to /home/user/~/project
    # and fails the transfer with exit 12. csync resolves it to the equivalent
    # relative path BEFORE anything reads it — rsync interprets a relative remote
    # path against the login home — and discloses the rewrite so it isn't silent.
    # Pull puts the ~ remote in the SOURCE position that gets echoed. The reported
    # source must be the stripped form AND the disclosure must name the original;
    # drop the normalization and the echoed source keeps the ~ (and no note
    # prints) — red.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:~/project"
      """
    When  I run "csync pull" from the project directory
    Then  the reported source should be "user@host:project"
    And   csync should report that it rewrote "~/project"

  Scenario: A saved remote with a ~user home shortcut is rejected
    # ~user names another user's home, which no relative path can reach — only an
    # absolute path does. Rather than let rsync fail confusingly mid-transfer
    # (exit 12, like #50), csync rejects it up front: non-zero, before any change
    # list, with an error that names the tilde so the user can see what to fix.
    # Remove the reject and csync hands the literal ~deploy to rsync, which fails
    # later with a different message — red.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:~deploy/project"
      """
    When  I run "csync pull" from the project directory
    Then  csync should return a non-zero exit code
    And   the reported error should mention "~"

  Scenario: A trailing slash on the saved remote is normalized away in the display
    # csync appends its own trailing slash to force directory-contents semantics;
    # a remote written WITH one would otherwise become a doubled slash that only
    # works because rsync tolerates it. csync owns the shape instead, collapsing
    # the slash so the echoed operand is clean. Pull puts the remote in the source
    # position. Remove the collapse and the echoed source keeps its trailing
    # slash — red.
    Given a local directory containing these files:
      """
      README.md
      """
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project/"
      """
    When  I run "csync pull" from the project directory
    Then  the reported source should be "user@host:/project"

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Source display for push: v1 shows the local operand as ".". Decide whether
  #   to show the resolved absolute cwd instead (clearer in logs, longer line).
  #
  # - Discovery is cwd-only in v1: running `csync push` from a SUBDIRECTORY of a
  #   project whose .csync.toml sits at the root must NOT find it (it errors).
  #   When walk-up discovery lands, this flips to "found from a subdirectory".
