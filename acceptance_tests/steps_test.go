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
	ctx.Step(`^the log should record that the \.git directory was excluded$`, theLogShouldRecordThatTheGitDirectoryWasExcluded)
	ctx.Step(`^the log should record that source path as one argument$`, theLogShouldRecordThatSourcePathAsOneArgument)
	ctx.Step(`^the log should record the command line that was run$`, theLogShouldRecordTheCommandLineThatWasRun)
	ctx.Step(`^the log should name the source and destination csync reported$`, theLogShouldNameTheSourceAndDestinationReported)
	ctx.Step(`^I answer the prompt$`, iAnswerThePrompt)
	// A restatement of `csync should return exit code 0` in the vocabulary of a
	// scenario that has no interest in the number, only in csync having finished
	// what it was doing rather than falling over partway.
	ctx.Step(`^csync should exit normally$`, csyncShouldExitNormally)
	ctx.Step(`^the reported log path should be the one I found earlier$`, theReportedLogPathShouldBeTheOneIFoundEarlier)
	ctx.Step(`^that csync cannot write its log$`, thatCsyncCannotWriteItsLog)
	ctx.Step(`^the changed file is deleted before I answer$`, theChangedFileIsDeletedBeforeIAnswer)
	ctx.Step(`^the changed file should be identical between local and remote$`, theChangedFileShouldBeIdenticalBetweenLocalAndRemote)
	ctx.Step(`^csync should warn that it could not write a run log$`, csyncShouldWarnThatItCouldNotWriteARunLog)
	ctx.Step(`^csync should not report where it logged the run$`, csyncShouldNotReportWhereItLoggedTheRun)
	ctx.Step(`^csync should say last of all that the run was not logged$`, csyncShouldSayLastOfAllThatTheRunWasNotLogged)
	ctx.Step(`^no run log should have been written$`, noRunLogShouldHaveBeenWritten)
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
	ctx.Step(`^that all of the files are identical between local and remote$`, allFilesIdenticalBetweenLocalAndRemote)
	ctx.Step(`^an empty remote directory$`, anEmptyRemoteDirectory)
	ctx.Step(`^a remote git repository containing these files:$`, aRemoteGitRepositoryContainingTheseFiles)
	ctx.Step(`^that the file "([^"]*)" has been changed locally$`, theFileHasBeenChangedLocally)
	ctx.Step(`^that the file "([^"]*)" has a different modification time but identical content$`, theFileHasADifferentMtimeButIdenticalContent)
	ctx.Step(`^that the file "([^"]*)" has been added locally$`, theFileHasBeenAddedLocally)
	ctx.Step(`^that the file "([^"]*)" has been added on the remote$`, theFileHasBeenAddedOnTheRemote)
	ctx.Step(`^that the file "([^"]*)" has been deleted locally$`, theFileHasBeenDeletedLocally)
	ctx.Step(`^no actions should be reported$`, noActionsShouldBeReported)
	ctx.Step(`^the reported actions should be:$`, theReportedActionsShouldBe)
	ctx.Step(`^the withheld changes should be:$`, theWithheldChangesShouldBe)
	ctx.Step(`^no withheld changes should be reported$`, noWithheldChangesShouldBeReported)
	ctx.Step(`^the reported actions should be, in order:$`, theReportedActionsShouldBeInOrder)
	ctx.Step(`^the reported changes should be numbered, in order:$`, theReportedChangesShouldBeNumberedInOrder)
	ctx.Step(`^the reported change count should be (\d+)$`, theReportedChangeCountShouldBe)
	ctx.Step(`^the reported excluded count should be (\d+)$`, theReportedExcludedCountShouldBe)
	ctx.Step(`^no gitignored paths should be reported as excluded$`, noGitignoredPathsShouldBeReportedAsExcluded)
	ctx.Step(`^the \.git directory should be reported as excluded$`, theGitDirectoryShouldBeReportedAsExcluded)
	ctx.Step(`^the \.csync\.toml file should be reported as excluded$`, theCsyncTomlShouldBeReportedAsExcluded)
	ctx.Step(`^the \.git directory should not be reported as excluded$`, theGitDirectoryShouldNotBeReportedAsExcluded)
	ctx.Step(`^the reported sync count should be (\d+)$`, theReportedSyncCountShouldBe)
	ctx.Step(`^the reported removed count should be (\d+)$`, theReportedRemovedCountShouldBe)
	ctx.Step(`^the file "([^"]*)" should be identical between local and remote$`, theFileShouldBeIdenticalBetweenLocalAndRemote)
	ctx.Step(`^the file "([^"]*)" should not exist on the remote$`, theFileShouldNotExistOnTheRemote)
	ctx.Step(`^the file "([^"]*)" should still exist on the remote$`, theFileShouldStillExistOnTheRemote)
	ctx.Step(`^the file "([^"]*)" should still differ between local and remote$`, theFileShouldStillDifferBetweenLocalAndRemote)

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		home, err := os.MkdirTemp("", "csync-home-*")
		if err != nil {
			return ctx, fmt.Errorf("mktempdir: %w", err)
		}
		ctx = context.WithValue(ctx, homeKey{}, home)
		for _, tag := range sc.Tags {
			if tag.Name == "@remote" {
				ctx = context.WithValue(ctx, remoteModeKey{}, true)
			}
		}
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
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
		localPath, _ := ctx.Value(localPathKey{}).(string)
		if localPath != "" {
			os.RemoveAll(localPath)
		}
		remotePath, _ := ctx.Value(remotePathKey{}).(string)
		if remotePath != "" {
			os.RemoveAll(remotePath)
		}
		home, _ := ctx.Value(homeKey{}).(string)
		if home != "" {
			os.RemoveAll(home)
		}
		return ctx, nil
	})
}
