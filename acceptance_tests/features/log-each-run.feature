Feature: Log each run

  In order to troubleshoot a sync after the fact — including one that removed
  files and so cannot be re-run to reproduce it — I want csync to record what it
  did to a log file on this machine, without my having to ask for it first.

  Background:
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """

  Scenario: A run that compares writes a run log and says where
    # The simplest run that still invokes rsync: nothing differs, so csync
    # reports no changes and returns without prompting for a selection.
    #
    # The scenario names no path. It learns where the log is from csync itself,
    # so nothing here has to know the layout of the state directory — that is
    # pinned once, by the location scenario below, and nowhere else in the suite.
    #
    # Exit code 0 is a guard rather than a second behavior. csync discloses the
    # log path on its failure paths too (those are the runs worth reading), so
    # the disclosure alone cannot tell a clean run from a broken one, and a run
    # that died after opening the log would otherwise satisfy this scenario.
    Given that all of the files are identical between local and remote
    When  I run "csync ./project user@host:/project"
    Then  csync should return exit code 0
    And   csync should report where it logged the run
    And   a run log should exist at the reported path

  Scenario: The log is written as csync runs, not when it ends
    # Records reach the disk as the run proceeds, not in one flush at the end.
    # csync can die without warning — a Ctrl-C at the prompt, a closed laptop, a
    # kill — and the runs worth reading are exactly the ones that ended badly. A
    # log assembled in memory and written on the way out is empty in every case it
    # exists for.
    #
    # The selection prompt is the observation point. csync blocks there on stdin,
    # after the comparison and before any transfer, so the log can be read mid-run
    # without racing it: the run is suspended, not merely slow.
    #
    # What is asserted is that a record is *there*, not what it says — the contents
    # are pinned one fact at a time by the scenarios below. Every one of those would
    # pass against a log flushed at exit. This one would not, and that is the whole
    # of its job.
    Given that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I look for the log file
    Then  the log file should already have content

  Scenario: csync reports the log it has been writing all along
    # The log csync names on its way out is the same file it was filling in while it
    # ran. Without this, the scenario above could be satisfied by any file that
    # happened to be lying around, and csync could disclose a path it never wrote
    # to — each half honest on its own, and useless together.
    #
    # csync discloses the path as it exits, which has not happened yet while it
    # waits at the prompt. So the log is found first and reconciled afterwards. No
    # path is named here either way: where the log belongs is pinned once, by the
    # location scenario below, and nowhere else in the suite.
    Given that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    And   I have taken note of where the log file is
    When  I answer the prompt
    Then  csync should exit normally
    And   the reported log path should be the one I found earlier

  Scenario: By the time it prompts, the log names the version that ran
    # Which build produced this run is the first thing a troubleshooter needs and
    # the thing a bug report most often omits. csync records it up front, before it
    # asks what to sync, so a run abandoned at the prompt still says which binary
    # made it. The prompt is the observation point for the same reason as the
    # write-as-you-go scenario: csync is suspended there, so the log can be read
    # without racing the run.
    #
    # The version is the known one the harness injects (see report-version); tying
    # the record to that literal is what proves csync logged its own version and
    # not a constant.
    Given that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I look for the log file
    Then  the log should record that the version was "0.0.0-test"

  Scenario: The log records the comparison csync ran
    # The comparison is the one external command every run makes, so it is where the
    # record of "what csync actually invoked" begins. A troubleshooter reading the log
    # can see the exact rsync that produced the change list — the dry-run that a
    # destructive run can no longer be repeated to reproduce.
    #
    # Read at the prompt, before any transfer, the comparison is the only command
    # that has run — so finding it there needs no other command to be filtered out.
    Given that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I look for the log file
    Then  the log should record running "rsync" for the comparison

  Scenario: The log records the command line as it was invoked
    # The literal invocation — what the user actually typed — heads the log, distinct
    # from the resolved source and destination below it. For an explicit run the two
    # look alike; the distinction earns its keep for a saved-target push or pull,
    # where the operands are derived and only this line still shows the verb.
    Given that all of the files are identical between local and remote
    When  I run "csync ./project user@host:/project"
    Then  csync should return exit code 0
    And   the log should record the command line that was run

  Scenario: The log names both operands of the run
    # A run's operands — what it compared, and in which direction — are the frame the
    # rest of the log hangs on. csync records both, and they are the same source and
    # destination it echoed in its header: the log agrees with what the user saw, so a
    # reader is never left guessing which way the sync went.
    #
    # The identical-pair setup is the simplest run that reaches rsync and returns
    # without prompting, so the whole run — header, log, disclosed path — is on hand
    # to reconcile once it exits. No path is named here; csync is the source of truth
    # for what the operands resolved to.
    Given that all of the files are identical between local and remote
    When  I run "csync ./project user@host:/project"
    Then  csync should return exit code 0
    And   the log should name the source and destination csync reported

  @remote
  Scenario: Under a saved-target push, the log keeps the verb and the resolved operands apart
    # This is where the invocation line earns its place. On an explicit run it and the
    # source/destination lines look alike; under `csync push` they diverge — the
    # invocation stays the verb the user typed, while source and destination are what
    # csync resolved that verb to from .csync.toml (here, "." and the saved remote).
    # A troubleshooter reading a "push went the wrong way" report needs both halves:
    # what was asked for, and what it became.
    Given that all of the files are identical between local and remote
    And   a ".csync.toml" in the project directory containing:
      """
      remote = "user@host:/project"
      """
    When  I run "csync push" from the project directory
    Then  csync should return exit code 0
    And   the log should record the command line that was run
    And   the log should name the source and destination csync reported

  Scenario: A log that cannot be written does not stop the sync
    # The record is a diagnostic, never a precondition. A state directory that has
    # gone read-only, or an XDG_STATE_HOME pointing at something that is not a
    # directory, says nothing about whether the files should move — and a tool that
    # refuses to work because it cannot keep a diary is a tool nobody keeps.
    Given that csync cannot write its log
    And   that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I answer the prompt
    Then  csync should exit normally
    And   the changed file should be identical between local and remote

  Scenario: csync warns when it cannot write a log
    # Silently declining to log would be worse than not logging: the user would go
    # looking for the record of a destructive run and find nothing, with no way to
    # know whether csync failed to write it or they had misremembered where it went.
    Given that csync cannot write its log
    And   that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I answer the prompt
    Then  csync should warn that it could not write a run log

  Scenario: A run that could not log says so again when it ends
    # The warning comes before csync asks what to sync, so the user can still stop.
    # But a long change list scrolls it away, and the interactive picker holds only
    # the rows it printed itself on screen — so by the time the run is over, the
    # notice may be gone. A run that logged nothing therefore says so once more as
    # it exits, in the same place a run that logged something names its file.
    Given that csync cannot write its log
    And   that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I answer the prompt
    Then  csync should say last of all that the run was not logged

  Scenario: csync names no log when it wrote none
    # csync discloses a path only when there is a file at the end of it. Printing
    # one here would send the user to a log that was never created, which is a
    # worse answer than admitting there is none.
    Given that csync cannot write its log
    And   that a file has been changed locally
    And   I have started csync but not yet answered the prompt
    When  I answer the prompt
    Then  csync should not report where it logged the run

  Scenario: --version writes no log
    # A run that only reports the version invokes no rsync, so it has nothing to
    # troubleshoot. It returns before the log is ever opened, and must not leave a
    # state directory behind — a shell completion or a package manager probing the
    # binary this way should cost nothing on disk.
    When I run "csync --version"
    Then no run log should have been written

  Scenario: --license writes no log
    # As with --version: the license text is printed and csync returns, before any
    # operand is resolved and before rsync runs.
    When I run "csync --license"
    Then no run log should have been written

  Scenario: A usage error writes no log
    # A run rejected for the wrong arguments never reaches rsync either, so it too
    # records nothing. The log is for troubleshooting a sync that happened, not a
    # command that was never valid.
    When I run "csync"
    Then no run log should have been written

  Scenario: A run rejected before it reaches rsync writes no log
    # A ~user home shortcut has no relative form, so csync rejects it as it
    # normalizes the operands — after the command parses, but before it opens a log
    # or runs rsync. The run fails and leaves nothing behind: the log is opened only
    # once csync knows there is a sync worth recording. This is the case that pins
    # that ordering; the others are caught earlier, at parse time.
    When I run "csync ./project host:~alice/x"
    Then no run log should have been written

  # These three scenarios are the only ones in the suite that name where the log
  # lives. Every other scenario learns the path from csync, so the state-directory
  # layout is pinned here and nowhere else — change it, and only this block moves.

  Scenario: The log is written under the XDG state directory
    # The identical-pair setup is scaffolding, not the point: it is the simplest
    # run that reaches rsync and so writes a log (nothing differs, so csync reports
    # no changes and returns without prompting). What is under test is that the log
    # honors XDG_STATE_HOME.
    Given the environment variable XDG_STATE_HOME is set
    And   that all of the files are identical between local and remote
    When  I run "csync ./project user@host:/project"
    Then  the run log should be under "cherry-sync" in $XDG_STATE_HOME

  Scenario: With no XDG_STATE_HOME, the log falls back to the home state directory
    # The variable most users never set. csync then keeps its logs where the XDG
    # base-directory spec says state belongs: ~/.local/state. This must be a path
    # distinct from the one above, or a csync that ignored XDG_STATE_HOME entirely
    # and always used the home fallback would pass the XDG scenario by accident.
    Given the environment variable XDG_STATE_HOME is not set
    And   that all of the files are identical between local and remote
    When  I run "csync ./project user@host:/project"
    Then  the run log should be under "cherry-sync" in ~/.local/state

  Scenario: The log is kept private to the user
    # The log names every path a run touched, which discloses the shape of the
    # user's work tree. Nothing outside the account has cause to read it, so the
    # directory and the file are the owner's alone. Never the project directory,
    # for a sharper reason: csync withholds only .csync.toml and .git from a
    # comparison, so a log written in-tree would show up as a change and be pushed
    # to the remote — but the scenarios above already pin it outside the project.
    Given that all of the files are identical between local and remote
    When  I run "csync ./project user@host:/project"
    Then  the run log directory should be accessible only by its owner
    And   the run log file should be accessible only by its owner

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, agreed but not yet drafted.
  # Each becomes a real Scenario block as we drill into it, one at a time.
  # ---------------------------------------------------------------------------
  #
  # - csync discloses the log path on every run that writes one: the no-changes
  #   run, the completed sync, and the runs that fail at the comparison or mid
  #   transfer. A record nobody can find is not a record.
  #
  # - One log per run, so a second run does not overwrite the first, and old
  #   logs are pruned rather than accumulating without bound.
  #
  # - By the time csync blocks at the selection prompt, the log names the run's
  #   start, csync's version, both operands, and the comparison rsync invoked.
  #   One scenario per fact; the scenario above already holds csync to writing
  #   them before it asks, rather than at the end.
  #
  # - Each external command csync runs — rsync for the comparison, the transfer,
  #   and the removal pass; git for the ignore rules — is recorded with its
  #   argument vector, its exit code, and how long it took.
  #
  # - A path containing a space comes back out of the log as a single argument,
  #   so a reader can tell where one operand ends and the next begins.
  #
  # - The actions csync classified, and which of them the user selected, are
  #   recorded — the two can differ, and a removal that was applied is the fact
  #   the log exists to preserve.
