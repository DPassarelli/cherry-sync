# @remote is required here, not incidental: a peer that can go silent only exists
# on rsync's sender/receiver code path, which a local-to-local run never takes.
#
# The remote shell this feature installs answers the comparison normally and goes
# silent only afterwards, so the stall lands on the transfer. That is deliberate:
# the transfer is the leg csync bounds with rsync's own --timeout, while the
# comparison is bounded by its wall clock instead. A comparison legitimately goes
# quiet for a long time (it hashes both sides), so bounding it on silence would
# report a healthy remote as a dead one.
@remote
Feature: Bound a stalled transfer

  In order to get an answer instead of an open-ended wait, I want csync to give
  up on a transfer whose remote has stopped responding, and to say that is what
  happened rather than leaving me to guess.

  Background:
    Given a local directory containing these files:
      """
      src/main.go
      README.md
      """
    And   that all of the files are identical between local and remote

  Scenario: A remote that goes silent during the transfer ends the run
    Given that the file "README.md" has been changed locally
    And   a remote that goes silent once the comparison is done
    When  I run "csync ./project user@host:/project" and respond with "<empty>"
    Then  the reported error should mention "stopped responding"
    And   csync should return exit code 1
