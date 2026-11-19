# Record decisions as dated files in docs/decisions

Status: Accepted

## Decision

Record every decision that spans several PRs or rules something out as its own file in `docs/decisions/`, named for the day it was decided, and shaped like the template in this folder. Do not rewrite a recorded decision; supersede it with a new record. Decisions recorded elsewhere before this convention migrate here over time, each leaving a link behind.

## Why

Decisions were spread across topic docs and issue threads, with no consistent place to state what was rejected or when to reconsider. That made settled questions easy to re-propose, by people and by agents alike. One folder with one shape gives a single place to check before proposing a design, and a test keeps that shape from drifting.

## Alternatives rejected

- **Keep decisions in the topic docs.** Each doc records them differently and few say when to revisit, so finding every relevant decision means reading everything.
- **Use closed issues as the log.** Issues are off disk, so they can't be read offline or reviewed in the same PR as the change they justify.
- **Number records sequentially.** Parallel branches collide on the next number, and migrated older decisions would carry high numbers with early dates, so neither order could be trusted.

## Revisit if

- The folder grows past what a date-sorted listing can navigate, and records need grouping by topic.
- The project adopts a tool that manages decision records itself.
- A decision has to be recorded after 2036-12-31, the latest date the guard test accepts.
