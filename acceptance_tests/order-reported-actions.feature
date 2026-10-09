Feature: Order the reported actions

  In order to scan the differences predictably and refer to them by number
  when selecting which to sync, I want the reported actions in a stable,
  human-friendly order — not rsync's raw emit order, which groups each
  directory's contents together so a nested file can appear far from where a
  reader scanning a flat list would expect it.

  # Contract: actions are ordered the way a file tree presents them, applying
  # these keys in order at each path level:
  #   1. dot entries (name begins with '.') before non-dot entries
  #   2. within each, files before subdirectories
  #   3. number-leading names before letter-leading names
  #   4. numbers compared by value (2 < 10), letters alphabetically
  #      (case-insensitive, byte order breaking case-only ties)

  Scenario: Reported actions are ordered like a file tree
    # A case-only tie (TODO.md vs. todo.md) is left out: the two names collapse to
    # one file on a case-insensitive filesystem such as macOS APFS, so a unit test
    # covers it instead.
    Given a local directory containing these files:
      """
      .gitignore
      .config/settings.toml
      01-setup.md
      10-data.csv
      2-config.yml
      LICENSE
      main.go
      README.md
      TODO.md
      src/adder.go
      src/parser.go
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the reported actions should be, in order:
      | action | path                  |
      | create | .gitignore            |
      | create | .config/settings.toml |
      | create | 01-setup.md           |
      | create | 2-config.yml          |
      | create | 10-data.csv           |
      | create | LICENSE               |
      | create | main.go               |
      | create | README.md             |
      | create | TODO.md               |
      | create | src/adder.go          |
      | create | src/parser.go         |

  Scenario: Each reported change is labeled with its selection number
    Given a local directory containing these files:
      """
      README.md
      src/adder.go
      """
    And   an empty remote directory
    When  I run "csync ./project user@host:/project"
    Then  the reported changes should be numbered, in order:
      | number | action | path         |
      | 1      | create | README.md    |
      | 2      | create | src/adder.go |

  # ---------------------------------------------------------------------------
  # TODO: Additional ordering scenarios, not yet drafted.
  # ---------------------------------------------------------------------------
  #
  # - Ordering is independent of verb: a fixture mixing creates, updates, and
  #   deletes is still sorted purely by path, not grouped by action.
