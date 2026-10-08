# Scenarios are brought into the run one at a time as we implement them. Each
# not-yet-implemented scenario carries its own @wip tag (excluded via the
# "~@wip" tag filter the runner applies); drop a scenario's tag when we drill in.
#
# @remote runs every scenario here over a fake SSH remote, so transfers run the
# way they would against a real host. A local-to-local run reports every change
# in the same direction, which once hid a push-direction bug.
@remote
Feature: Select and sync files

  In order to move only the files I actually want, I want to review the
  differences, pick which ones to transfer, and have csync sync exactly those.

  Background:
    Given a local directory containing these files:
      """
      src/main.go
      src/parser.go
      README.md
      LICENSE
      .gitignore
      """

  Scenario: No differences — nothing to sync, no prompt
    When  I run "csync ./project user@host:/project"
    Then  the reported message should begin with "No changes"
    And   csync should return exit code 0

  Scenario: Accepting the default selects every change
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "<empty>"
    Then  the files should end up:
      | path         | state   |
      | README.md    | in sync |
      | src/adder.go | in sync |

  Scenario: Selecting "a" selects every change
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    Then  the files should end up:
      | path         | state   |
      | README.md    | in sync |
      | src/adder.go | in sync |

  Scenario: A filename containing non-ASCII bytes transfers intact
    # This first surfaced with a macOS screenshot, whose name carries a narrow
    # no-break space (U+202F).
    Given that the file "café.txt" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    Then  the files should end up:
      | path     | state   |
      | café.txt | in sync |

  Scenario: A completed sync leaves nothing to re-sync
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    And   I run "csync ./project user@host:/project" a second time
    Then  no actions should be reported

  Scenario: Choosing a subset by number syncs only those files
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "1"
    Then  the files should end up:
      | path         | state       |
      | README.md    | in sync     |
      | src/adder.go | out of sync |

  Scenario: A different number selects a different change
    Given that the file "README.md" has been changed locally
    And   that the file "src/adder.go" has been added locally
    When  I run "csync ./project user@host:/project" and respond with "2"
    Then  the files should end up:
      | path         | state       |
      | src/adder.go | in sync     |
      | README.md    | out of sync |

  Scenario: Choosing none transfers nothing and exits cleanly
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "n"
    Then  the files should end up:
      | path      | state       |
      | README.md | out of sync |
    And   csync should return exit code 0

  Scenario: An unrecognized response is rejected without transferring
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "wat"
    Then  csync should return a non-zero exit code
    And   the files should end up:
      | path      | state       |
      | README.md | out of sync |

  Scenario: An out-of-range number is rejected like an unrecognized response
    Given that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "2"
    Then  csync should return a non-zero exit code
    And   the files should end up:
      | path      | state       |
      | README.md | out of sync |

  Scenario: A hyphen range selects an inclusive span of changes
    # Rows number in tree order: 1 LICENSE, 2 README.md, 3 src/main.go, 4 src/parser.go.
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    And   that the file "src/parser.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1-3"
    Then  the files should end up:
      | path          | state       |
      | LICENSE       | in sync     |
      | README.md     | in sync     |
      | src/main.go   | in sync     |
      | src/parser.go | out of sync |

  Scenario: A comma list selects exactly the named changes
    # Rows number in tree order: 1 LICENSE, 2 README.md, 3 src/main.go.
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1,3"
    Then  the files should end up:
      | path        | state       |
      | LICENSE     | in sync     |
      | src/main.go | in sync     |
      | README.md   | out of sync |

  Scenario: A combined range and list selects the span plus the named change
    # Rows number in tree order: 1 LICENSE, 2 README.md, 3 src/main.go, 4 src/parser.go.
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    And   that the file "src/parser.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1-2,4"
    Then  the files should end up:
      | path          | state       |
      | LICENSE       | in sync     |
      | README.md     | in sync     |
      | src/parser.go | in sync     |
      | src/main.go   | out of sync |

  Scenario: Overlapping members are synced once, not twice
    # Rows number in tree order: 1 LICENSE, 2 README.md, 3 src/main.go.
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1-3,2"
    Then  the files should end up:
      | path        | state   |
      | LICENSE     | in sync |
      | README.md   | in sync |
      | src/main.go | in sync |

  Scenario: An out-of-range member rejects the whole selection
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1-3"
    Then  csync should return a non-zero exit code
    And   the files should end up:
      | path      | state       |
      | LICENSE   | out of sync |
      | README.md | out of sync |

  Scenario: A reversed range is rejected
    # It is not silently reordered to "1-3".
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "3-1"
    Then  csync should return a non-zero exit code
    And   the files should end up:
      | path        | state       |
      | LICENSE     | out of sync |
      | README.md   | out of sync |
      | src/main.go | out of sync |

  Scenario: Whitespace around members and range operands is ignored
    # Rows number in tree order: 1 LICENSE, 2 README.md, 3 src/main.go, 4 src/parser.go.
    Given that the file "LICENSE" has been changed locally
    And   that the file "README.md" has been changed locally
    And   that the file "src/main.go" has been changed locally
    And   that the file "src/parser.go" has been changed locally
    When  I run "csync ./project user@host:/project" and respond with "1 - 2, 4"
    Then  the files should end up:
      | path          | state       |
      | LICENSE       | in sync     |
      | README.md     | in sync     |
      | src/parser.go | in sync     |
      | src/main.go   | out of sync |

  Scenario: A file removed on the source is reported as a deletion
    # No selection is made, so the deletion is reported but not applied.
    Given that the file "README.md" has been deleted locally
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be:
      | action | path      |
      | delete | README.md |
    And   the files should end up:
      | path      | state       |
      | README.md | out of sync |

  Scenario: A selected deletion is applied to the destination
    # A nested file shows the removal reaches inside folders.
    Given that the file "src/parser.go" has been deleted locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    Then  the files should end up:
      | path          | state   |
      | src/parser.go | in sync |

  Scenario: Declining a deletion leaves the file on the destination
    Given that the file "README.md" has been deleted locally
    When  I run "csync ./project user@host:/project" and respond with "n"
    Then  the files should end up:
      | path      | state       |
      | README.md | out of sync |

  Scenario: A run mixing a transfer and a deletion applies and reports both
    # The summary calls out the removals, so "2 files total" doesn't read as two
    # transfers.
    Given that the file "src/adder.go" has been added locally
    And   that the file "README.md" has been deleted locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    Then  the files should end up:
      | path         | state   |
      | src/adder.go | in sync |
      | README.md    | in sync |
    And   the reported removed count should be 1

  Scenario: Selecting only the transfer leaves the deletion unapplied
    # Rows number in tree order: 1 adder.go, 2 README.md (the deletion).
    Given that the file "adder.go" has been added locally
    And   that the file "README.md" has been deleted locally
    When  I run "csync ./project user@host:/project" and respond with "1"
    Then  the files should end up:
      | path      | state       |
      | adder.go  | in sync     |
      | README.md | out of sync |

  Scenario: A completed deletion leaves nothing to re-sync
    Given that the file "README.md" has been deleted locally
    When  I run "csync ./project user@host:/project" and respond with "a"
    And   I run "csync ./project user@host:/project" a second time
    Then  no actions should be reported

  @wip
  Scenario: Pull direction — a remote-new file is brought down when selected
    Given that the file "notes.txt" has been added on the remote
    When  I run "csync user@host:/project ./project" and respond with "a"
    Then  the files should end up:
      | path      | state   |
      | notes.txt | in sync |

  # ---------------------------------------------------------------------------
  # TODO: Additional scenarios for this feature, not yet drafted.
  # Each will become a real Scenario block as we drill into it.
  # ---------------------------------------------------------------------------
  #
  # - Non-interactive escape hatch: a future --all/-y flag skips the prompt and
  #   syncs everything. Tied to the tabled flag-parsing work; out of MVP scope.
