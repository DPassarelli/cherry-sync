# Use CodeRabbit as an advisory reviewer

Status: Accepted

## Decision

Review every pull request with CodeRabbit before merging, as advice only. Do not make its review a required check, and do not let it, or any agent, merge.

## Why

Every pull request was authored and merged by one person, with no second reader. An AI reviewer from a different vendor than the one writing most of the code adds an independent view without sharing its blind spots. CodeRabbit is free for public repositories and reads CLAUDE.md as its rulebook on its own, so the rules stay in one place. Its project-specific guidance covers what CLAUDE.md cannot: scrutinizing tests and specs that change in the same pull request as the code they check.

The accepted cost: while the repository has fewer than 10 stars, CodeRabbit reviews only when asked, so each pull request needs a `@coderabbitai review` comment. Keeping it out of required checks means its rate limit can never block a merge.

## Alternatives rejected

- **GitHub Copilot code review.** It needs a paid Copilot plan and is billed per review.
- **Gemini Code Assist.** It takes its rules from its own style guide file, which would be a second copy of CLAUDE.md.
- **A Claude review in GitHub Actions.** The same model family would be reviewing its own work, and it needs an API key held as a repository secret.
- **Local review only, before pushing.** Nothing is left on the pull request, and it is easy to skip.

## Revisit if

- The repository reaches 10 stars, so reviews run automatically and the manual trigger can be dropped.
- CodeRabbit's open-source tier stops being free, or stops reading CLAUDE.md.
- Its reviews prove to be mostly noise, judged over a run of pull requests.
