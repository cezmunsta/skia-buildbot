// capture_archives_on_workers is an application that captures archives on all CT
// workers and uploads it to Google Storage. The requester is emailed when the task
// is done.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.skia.org/infra/ct/go/master_scripts/master_common"
	"go.skia.org/infra/ct/go/util"
	"go.skia.org/infra/go/git"
	"go.skia.org/infra/go/skerr"
	"go.skia.org/infra/go/sklog"
	skutil "go.skia.org/infra/go/util"
)

var (
	pagesetType = flag.String("pageset_type", "", "The type of pagesets to use. Eg: 10k, Mobile10k, All.")
	runOnGCE    = flag.Bool("run_on_gce", true, "Run on Linux GCE instances.")
	runID       = flag.String("run_id", "", "The unique run id (typically requester + timestamp).")
)

const (
	MAX_PAGES_PER_SWARMING_BOT = 100
)

func captureArchivesOnWorkers() error {
	master_common.Init("capture_archives")

	ctx := context.Background()

	// Finish with glog flush and how long the task took.
	defer util.TimeTrack(time.Now(), "Capture archives on Workers")
	defer sklog.Flush()

	if *pagesetType == "" {
		return errors.New("Must specify --pageset_type")
	}

	// Empty the remote dir before the workers upload to it.
	gs, err := util.NewGcsUtil(nil)
	if err != nil {
		return err
	}
	gsBaseDir := filepath.Join(util.SWARMING_DIR_NAME, util.WEB_ARCHIVES_DIR_NAME, *pagesetType)
	skutil.LogErr(gs.DeleteRemoteDir(gsBaseDir))

	// Find which chromium hash the workers should use.
	gitExec, err := git.Executable(ctx)
	if err != nil {
		return skerr.Wrap(err)
	}
	chromiumHash, err := util.GetChromiumHash(ctx, gitExec)
	if err != nil {
		return fmt.Errorf("Could not find the latest chromium hash: %s", err)
	}

	// Trigger task to return hash of telemetry isolates.
	telemetryHash, err := util.TriggerIsolateTelemetrySwarmingTask(ctx, "isolate_telemetry", *runID, chromiumHash, "", util.PLATFORM_LINUX, []string{}, 1*time.Hour, 1*time.Hour, *master_common.Local)
	if err != nil {
		return fmt.Errorf("Error encountered when swarming isolate telemetry task: %s", err)
	}
	if telemetryHash == "" {
		return errors.New("Found empty telemetry hash!")
	}
	isolateDeps := []string{telemetryHash}

	// Archive, trigger and collect swarming tasks.
	if _, err := util.TriggerSwarmingTask(ctx, *pagesetType, "capture_archives", util.CAPTURE_ARCHIVES_ISOLATE, *runID, "", util.PLATFORM_LINUX, 4*time.Hour, 1*time.Hour, util.TASKS_PRIORITY_LOW, MAX_PAGES_PER_SWARMING_BOT, util.PagesetTypeToInfo[*pagesetType].NumPages, map[string]string{}, *runOnGCE, *master_common.Local, 1, isolateDeps); err != nil {
		return fmt.Errorf("Error encountered when swarming tasks: %s", err)
	}

	return nil
}

func main() {
	retCode := 0
	if err := captureArchivesOnWorkers(); err != nil {
		sklog.Errorf("Error while running capture archives on workers: %s", err)
		retCode = 255
	}
	os.Exit(retCode)
}
