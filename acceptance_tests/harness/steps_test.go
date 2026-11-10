// The step registry: the single place a Gherkin phrase is bound to the Go
// function that implements it. Kept apart from the step bodies so the suite's
// whole vocabulary can be read without scrolling past their implementations.

package acceptance_test

import (
	"context"
	"fmt"
	"os"

	"github.com/cucumber/godog"
)

// InitializeScenario registers each Gherkin step with its step function and
// installs an After hook that removes the per-scenario local and remote
// tempdirs.
func InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.Step(`^I run "([^"]*)"$`, iRun)
	ctx.Step(`^I run "([^"]*)" a second time$`, iRun)
	ctx.Step(`^I run "([^"]*)" and respond with "([^"]*)"$`, iRunAndRespond)
	ctx.Step(`^I run "([^"]*)" from the project directory$`, iRunFromTheProjectDirectory)
	ctx.Step(`^I run "([^"]*)" from the project directory and respond with "([^"]*)"$`, iRunFromTheProjectDirectoryAndRespond)
	ctx.Step(`^the reported source should be "([^"]*)"$`, theReportedSourceShouldBe)
	ctx.Step(`^the reported destination should be "([^"]*)"$`, theReportedDestinationShouldBe)
	ctx.Step(`^csync should return exit code (\d+)$`, csyncShouldReturnExitCode)
	ctx.Step(`^csync should return a non-zero exit code$`, csyncShouldReturnANonZeroExitCode)
	ctx.Step(`^csync should report the diagnostic rsync wrote$`, csyncShouldReportTheDiagnosticRsyncWrote)
	ctx.Step(`^the help text should contain "([^"]*)"$`, theHelpTextShouldContain)
	ctx.Step(`^the reported message should begin with "([^"]*)"$`, theReportedMessageShouldBeginWith)
	ctx.Step(`^the reported version should be "([^"]*)"$`, theReportedVersionShouldBe)
	ctx.Step(`^the reported license should contain "([^"]*)"$`, theReportedLicenseShouldContain)
	ctx.Step(`^csync should report where it logged the run$`, csyncShouldReportWhereItLoggedTheRun)
	ctx.Step(`^csync should report where it logged the failed run$`, csyncShouldReportWhereItLoggedTheFailedRun)
	ctx.Step(`^a run log should exist at the reported path$`, aRunLogShouldExistAtTheReportedPath)
	ctx.Step(`^that a file has been changed locally$`, thatAFileHasBeenChangedLocally)
	ctx.Step(`^a remote that goes silent once the comparison is done$`, aRemoteThatGoesSilentOnceTheComparisonIsDone)
	ctx.Step(`^that the local copy of "([^"]*)" is (\d+) KB larger than the remote copy$`, theLocalCopyIsLarger)
	ctx.Step(`^that the two copies of "([^"]*)" differ in content but not in size$`, theTwoCopiesDifferInContentButNotSize)
	ctx.Step(`^that the (local|remote) copy of "([^"]*)" was last modified (\d+) (second|minute|hour|day)s? ago$`, theCopyWasLastModified)
	ctx.Step(`^that both copies of "([^"]*)" carry the same modification time$`, bothCopiesCarryTheSameModificationTime)
	ctx.Step(`^a remote that answers the comparison but fails the measurement$`, aRemoteThatFailsTheMeasurement)
	ctx.Step(`^the reported detail for "([^"]*)" should be "([^"]*)"$`, theReportedDetailForShouldBe)
	ctx.Step(`^no detail should be reported for "([^"]*)"$`, noDetailShouldBeReportedFor)
	ctx.Step(`^I have started csync but not yet answered the prompt$`, iHaveStartedCsyncButNotYetAnsweredThePrompt)
	// Two phrasings of the same act, kept apart because Gherkin reads better when a
	// Given narrates in the past and a When in the present. Both locate the log.
	ctx.Step(`^I look for the log file$`, iLocateTheLogFile)
	ctx.Step(`^I have taken note of where the log file is$`, iLocateTheLogFile)
	ctx.Step(`^the log file should already have content$`, theLogFileShouldAlreadyHaveContent)
	ctx.Step(`^the log should record that the version was "([^"]*)"$`, theLogShouldRecordThatTheVersionWas)
	ctx.Step(`^the log should record running "([^"]*)" for the comparison$`, theLogShouldRecordRunningForTheComparison)
	ctx.Step(`^the log should record the transfer that ran$`, theLogShouldRecordTheTransferThatRan)
	ctx.Step(`^the log should record the removal that ran$`, theLogShouldRecordTheRemovalThatRan)
	ctx.Step(`^the log should record running "([^"]*)" for the ignore rules$`, theLogShouldRecordRunningForTheIgnoreRules)
	ctx.Step(`^a local directory whose path contains a space$`, aLocalDirectoryWhosePathContainsASpace)
	ctx.Step(`^a local directory whose path contains a double quote$`, aLocalDirectoryWhosePathContainsADoubleQuote)
	ctx.Step(`^a local source path that does not exist$`, aLocalSourcePathThatDoesNotExist)
	ctx.Step(`^the log should record the comparison's failing exit code$`, theLogShouldRecordTheComparisonsFailingExitCode)
	ctx.Step(`^the log should record what rsync said about the failure$`, theLogShouldRecordWhatRsyncSaidAboutTheFailure)
	ctx.Step(`^the logged duration should be a positive whole number of milliseconds$`, theLoggedDurationShouldBeAPositiveWholeNumberOfMilliseconds)
	ctx.Step(`^the log should record (\d+) classified changes?$`, theLogShouldRecordNClassifiedChanges)
	ctx.Step(`^the log should record (\d+) selected changes?$`, theLogShouldRecordNSelectedChanges)
	ctx.Step(`^the classified changes should include "([^"]*)" of "([^"]*)"$`, theClassifiedChangesShouldInclude)
	ctx.Step(`^the selected changes should include "([^"]*)" of "([^"]*)"$`, theSelectedChangesShouldInclude)
	ctx.Step(`^the log should record "([^"]*)" among the excluded paths$`, theLogShouldRecordAmongTheExcludedPaths)
	ctx.Step(`^the log should record the excluded paths:$`, theLogShouldRecordTheExcludedPaths)
	ctx.Step(`^the log should record (\d+) excluded paths$`, theLogShouldRecordExcludedPaths)
	ctx.Step(`^the log should record that the \.git directory was excluded$`, theLogShouldRecordThatTheGitDirectoryWasExcluded)
	ctx.Step(`^the log should record that source path as one argument$`, theLogShouldRecordThatSourcePathAsOneArgument)
	ctx.Step(`^the log should record the command line that was run$`, theLogShouldRecordTheCommandLineThatWasRun)
	ctx.Step(`^the log should name the source and destination csync reported$`, theLogShouldNameTheSourceAndDestinationReported)
	ctx.Step(`^I answer the prompt$`, iAnswerThePrompt)
	// A restatement of `csync should return exit code 0` in the vocabulary of a
	// scenario that has no interest in the number, only in csync having finished
	// what it was doing rather than falling over partway.
	ctx.Step(`^the reported log path should be the one I found earlier$`, theReportedLogPathShouldBeTheOneIFoundEarlier)
	ctx.Step(`^that csync cannot write its log$`, thatCsyncCannotWriteItsLog)
	ctx.Step(`^the changed file is deleted before I answer$`, theChangedFileIsDeletedBeforeIAnswer)
	ctx.Step(`^the changed file should be identical between local and remote$`, theChangedFileShouldBeIdenticalBetweenLocalAndRemote)
	ctx.Step(`^csync should warn that it could not write a run log$`, csyncShouldWarnThatItCouldNotWriteARunLog)
	ctx.Step(`^csync should not report where it logged the run$`, csyncShouldNotReportWhereItLoggedTheRun)
	ctx.Step(`^csync should say last of all that the run was not logged$`, csyncShouldSayLastOfAllThatTheRunWasNotLogged)
	ctx.Step(`^no run log should have been written$`, noRunLogShouldHaveBeenWritten)
	ctx.Step(`^no run log should have been written for the rejected run$`, noRunLogShouldHaveBeenWrittenForTheRejectedRun)
	ctx.Step(`^the environment variable XDG_STATE_HOME is set$`, xdgStateHomeIsSet)
	ctx.Step(`^the environment variable XDG_STATE_HOME is not set$`, xdgStateHomeIsNotSet)
	ctx.Step(`^the run log should be under "([^"]*)" in (.+)$`, theRunLogShouldBeUnderIn)
	ctx.Step(`^the run log directory should be accessible only by its owner$`, theRunLogDirectoryShouldBeAccessibleOnlyByItsOwner)
	ctx.Step(`^the run log file should be accessible only by its owner$`, theRunLogFileShouldBeAccessibleOnlyByItsOwner)
	ctx.Step(`^(\d+) run logs already exist$`, nRunLogsAlreadyExist)
	ctx.Step(`^a file that is not a run log in the log directory$`, aFileThatIsNotARunLogInTheLogDirectory)
	ctx.Step(`^the log directory should hold (\d+) run logs?$`, theLogDirectoryShouldHoldNRunLogs)
	ctx.Step(`^the surviving run logs should be the newest ones$`, theSurvivingRunLogsShouldBeTheNewestOnes)
	ctx.Step(`^that file should still be there$`, thatFileShouldStillBeThere)
	ctx.Step(`^the log should name the run logs it pruned$`, theLogShouldNameTheRunLogsItPruned)
	ctx.Step(`^the log should record that nothing was pruned$`, theLogShouldRecordThatNothingWasPruned)
	ctx.Step(`^the reported error should mention "([^"]*)"$`, theReportedErrorShouldMention)
	ctx.Step(`^csync should report that it rewrote "([^"]*)"$`, csyncShouldReportThatItRewrote)
	ctx.Step(`^a local directory containing these files:$`, aLocalDirectoryContainingTheseFiles)
	ctx.Step(`^a local directory in the home directory containing these files:$`, aLocalDirectoryInTheHomeDirectoryContainingTheseFiles)
	ctx.Step(`^a "\.csync\.toml" in the project directory containing:$`, aCsyncTomlInTheProjectDirectoryContaining)
	ctx.Step(`^a local git repository containing these files:$`, aLocalGitRepositoryContainingTheseFiles)
	ctx.Step(`^the repository's "([^"]*)" contains:$`, theLocalFileContains)
	ctx.Step(`^the directory's "([^"]*)" contains:$`, theLocalFileContains)
	ctx.Step(`^an empty remote directory$`, anEmptyRemoteDirectory)
	ctx.Step(`^a remote git repository containing these files:$`, aRemoteGitRepositoryContainingTheseFiles)
	ctx.Step(`^that the file "([^"]*)" has been changed locally$`, theFileHasBeenChangedLocally)
	ctx.Step(`^that the file "([^"]*)" has a different modification time but identical content$`, theFileHasADifferentMtimeButIdenticalContent)
	ctx.Step(`^that the file "([^"]*)" has been added locally$`, theFileHasBeenAddedLocally)
	ctx.Step(`^that the file "([^"]*)" has been added on the remote$`, theFileHasBeenAddedOnTheRemote)
	ctx.Step(`^that (\d+) files have been added on the remote under "([^"]*)"$`, filesHaveBeenAddedOnTheRemoteUnder)
	ctx.Step(`^that (\d+) files have been added locally under "([^"]*)"$`, filesHaveBeenAddedLocallyUnder)
	ctx.Step(`^that the file "([^"]*)" has been deleted locally$`, theFileHasBeenDeletedLocally)
	ctx.Step(`^no actions should be reported$`, noActionsShouldBeReported)
	ctx.Step(`^the reported actions should be:$`, theReportedActionsShouldBe)
	ctx.Step(`^the withheld changes should be:$`, theWithheldChangesShouldBe)
	ctx.Step(`^no withheld changes should be reported$`, noWithheldChangesShouldBeReported)
	ctx.Step(`^the withheld changes should be summarized as:$`, theWithheldChangesShouldBeSummarizedAs)
	ctx.Step(`^the withheld changes should list (\d+) individual files$`, theWithheldChangesShouldListIndividualFiles)
	ctx.Step(`^the reported actions should be, in order:$`, theReportedActionsShouldBeInOrder)
	ctx.Step(`^the reported changes should be numbered, in order:$`, theReportedChangesShouldBeNumberedInOrder)
	ctx.Step(`^the \.git directory should be reported as excluded$`, theGitDirectoryShouldBeReportedAsExcluded)
	ctx.Step(`^the \.csync\.toml file should be reported as excluded$`, theCsyncTomlShouldBeReportedAsExcluded)
	ctx.Step(`^the \.git directory should not be reported as excluded$`, theGitDirectoryShouldNotBeReportedAsExcluded)
	ctx.Step(`^the reported sync count should be (\d+)$`, theReportedSyncCountShouldBe)
	ctx.Step(`^the files should end up:$`, theFilesShouldEndUp)
	ctx.Step(`^the reported removed count should be (\d+)$`, theReportedRemovedCountShouldBe)

	ctx.Step(`^that the file "([^"]*)" has been changed locally from "([^"]*)" to "([^"]*)"$`, theFileHasBeenChangedLocallyFromTo)
	ctx.Step(`^that (the local copy|the remote copy|both copies) of "([^"]*)" has been changed to (\d{3},\d{3}) bytes$`, theCopiesHaveBeenChangedToSize)
	ctx.Step(`^that (the local copy|the remote copy|both copies) of "([^"]*)" has been changed to contain a NUL byte$`, theCopiesHaveBeenChangedToContainANulByte)
	ctx.Step(`^that I have run "([^"]*)" in a terminal$`, thatIHaveRunInATerminal)
	ctx.Step(`^a remote that answers the comparison but is slow to send files$`, aRemoteThatIsSlowToSendFiles)
	ctx.Step(`^a remote that answers the comparison but fails to send files$`, aRemoteThatFailsToSendFiles)
	ctx.Step(`^the diff overlay should begin with "([^"]*)"$`, theDiffOverlayShouldBeginWithText)
	ctx.Step(`^the diff overlay should include what the remote said about the failure$`, theDiffOverlayShouldIncludeWhatTheRemoteSaid)
	ctx.Step(`^a remote that fails to send files with a message wider than the terminal$`, aRemoteThatFailsToSendFilesWithALongMessage)
	ctx.Step(`^the diff overlay should include all of what the remote said$`, theDiffOverlayShouldIncludeAllOfWhatTheRemoteSaid)
	ctx.Step(`^that the file "([^"]*)" has been changed locally to a single line wider than the terminal$`, theFileHasBeenChangedLocallyToASingleWideLine)
	ctx.Step(`^the diff overlay should include all of that line$`, theDiffOverlayShouldIncludeAllOfThatLine)
	ctx.Step(`^the row after that line's first row should begin with "([^"]*)"$`, theRowAfterThatLinesFirstRowShouldBeginWith)
	ctx.Step(`^that the file "([^"]*)" has been changed locally to the line "([^"]*)"$`, theFileHasBeenChangedLocallyToTheLine)
	ctx.Step(`^the diff overlay should include the line "([^"]*)"$`, theDiffOverlayShouldIncludeTheLine)
	ctx.Step(`^I run "([^"]*)" in a terminal$`, thatIHaveRunInATerminal)
	ctx.Step(`^the hint line should read "([^"]*)"$`, theHintLineShouldRead)
	ctx.Step(`^I press "([^"]*)" on the row for "([^"]*)"$`, iPressOnTheRowFor)
	ctx.Step(`^that I have pressed "([^"]*)" on the row for "([^"]*)"$`, thatIHavePressedOnTheRowFor)
	ctx.Step(`^I press "([^"]*)"$`, iPress)
	ctx.Step(`^that I have pressed "([^"]*)"$`, thatIHavePressed)
	ctx.Step(`^the diff overlay should no longer be visible$`, theDiffOverlayShouldNoLongerBeVisible)
	ctx.Step(`^the source and destination should be on screen$`, theSourceAndDestinationShouldBeOnScreen)
	ctx.Step(`^the notice should no longer be visible$`, theNoticeShouldNoLongerBeVisible)
	ctx.Step(`^the cursor should be on the row for "([^"]*)"$`, theCursorShouldBeOnTheRowFor)
	ctx.Step(`^that the file "([^"]*)" has been changed locally so that its diff is taller than the terminal$`, theFileHasBeenChangedSoThatItsDiffIsTaller)
	ctx.Step(`^I press "([^"]*)" more times than the diff has lines$`, iPressMoreTimesThanTheDiffHasLines)
	ctx.Step(`^that I have pressed "([^"]*)" more times than the diff has lines$`, thatIHavePressedMoreTimesThanTheDiffHasLines)
	ctx.Step(`^the diff overlay should show its last line$`, theDiffOverlayShouldShowItsLastLine)
	ctx.Step(`^the diff overlay should not show its last line$`, theDiffOverlayShouldNotShowItsLastLine)
	ctx.Step(`^that the diff overlay does not show its last line$`, theDiffOverlayShouldNotShowItsLastLine)
	ctx.Step(`^the diff overlay should still be open$`, theDiffOverlayShouldStillBeOpen)
	ctx.Step(`^no files should have been transferred$`, noFilesShouldHaveBeenTransferred)
	ctx.Step(`^the row for "([^"]*)" should be checked$`, theRowShouldBeChecked)
	ctx.Step(`^csync should have exited$`, csyncShouldHaveExited)
	ctx.Step(`^the diff overlay should include:$`, theDiffOverlayShouldInclude)
	ctx.Step(`^the diff overlay's title should read "([^"]*)"$`, theDiffOverlaysTitleShouldRead)
	ctx.Step(`^the diff overlay's header should read:$`, theDiffOverlaysHeaderShouldRead)
	ctx.Step(`^the diff overlay should be drawn inside a box$`, theDiffOverlayShouldBeDrawnInsideABox)
	ctx.Step(`^the lines beginning "([^"]*)" and "([^"]*)" should be (red|green)$`, theLinesBeginningShouldBe)
	ctx.Step(`^the picker should note "([^"]*)"$`, thePickerShouldNote)
	ctx.Step(`^the picker's note should contain "([^"]*)"$`, thePickersNoteShouldContain)
	ctx.Step(`^the diff hint should read "([^"]*)"$`, theDiffHintShouldRead)
	ctx.Step(`^a remote that records each file it sends$`, aRemoteThatRecordsEachFileItSends)
	ctx.Step(`^the remote should not have been asked for a copy$`, theRemoteShouldNotHaveBeenAskedForACopy)
	ctx.Step(`^the note "([^"]*)" should be bold$`, theNoteShouldBeBold)
	ctx.Step(`^the bold text in the detail should be "([^"]*)"$`, theBoldTextInTheDetailShouldBe)
	ctx.Step(`^the bold text in the detail should be "([^"]*)" and "([^"]*)"$`, theBoldTextInTheDetailShouldBeBoth)
	ctx.Step(`^no part of the detail should be bold$`, noPartOfTheDetailShouldBeBold)

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		home, err := os.MkdirTemp("", "csync-home-*")
		if err != nil {
			return ctx, fmt.Errorf("mktempdir: %w", err)
		}
		ctx = context.WithValue(ctx, homeKey{}, home)
		ctx = context.WithValue(ctx, exitCheckKey{}, &exitCheck{})
		for _, tag := range sc.Tags {
			if tag.Name == "@remote" {
				ctx = context.WithValue(ctx, remoteModeKey{}, true)
			}
		}
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, stepErr error) (context.Context, error) {
		// Reap a csync left blocked at the prompt — by a scenario that means to leave
		// it there, or by one that failed before it could answer. Without this, the
		// tempdirs below are removed out from under a live process and `go test` waits
		// forever on a child waiting for a stdin nobody will write to.
		run, _ := ctx.Value(startedKey{}).(*runningCsync)
		if run != nil && !run.reaped {
			run.stdin.Close()
			run.cmd.Process.Kill()
			<-run.done
		}
		session, _ := ctx.Value(terminalSessionKey{}).(*terminalSession)
		if session != nil {
			session.close()
		}
		localPath, _ := ctx.Value(localPathKey{}).(string)
		if localPath != "" {
			os.RemoveAll(localPath)
		}
		remoteHome, _ := ctx.Value(remoteHomeKey{}).(string)
		if remoteHome != "" {
			os.RemoveAll(remoteHome)
		}
		home, _ := ctx.Value(homeKey{}).(string)
		if home != "" {
			os.RemoveAll(home)
		}
		if stepErr != nil {
			return ctx, nil
		}
		return ctx, unassertedFailure(ctx)
	})
}
