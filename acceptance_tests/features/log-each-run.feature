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

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, agreed but not yet drafted.
  # Each becomes a real Scenario block as we drill into it, one at a time.
  # ---------------------------------------------------------------------------
  #
  # - csync discloses the log path on every run that writes one: the no-changes
  #   run, the completed sync, and the runs that fail at the comparison or mid
  #   transfer. A record nobody can find is not a record.
  #
  # - "csync --version" and "csync --license" write no run log, and neither does
  #   a usage error. All three return before any operand is resolved and before
  #   rsync runs, so there is nothing to troubleshoot — and a shell-completion
  #   probe must not litter the state directory.
  #
  # - The log is written under $XDG_STATE_HOME/cherry-sync/, falling back to
  #   ~/.local/state/cherry-sync/ when that variable is unset. This is the only
  #   scenario that may name the path. Never the project directory: csync
  #   withholds only .csync.toml and .git from a comparison, so an in-tree log
  #   would show up as a change and be pushed to the remote.
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
  #
  # - A run log that cannot be written (a read-only state directory, or
  #   XDG_STATE_HOME naming something that is not one) warns once on stderr and
  #   does not fail the sync. The record is a diagnostic, never a precondition.
