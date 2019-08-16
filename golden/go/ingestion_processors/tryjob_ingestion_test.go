package ingestion_processors

import (
	"context"
	"fmt"
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"
	"go.skia.org/infra/go/ds"
	"go.skia.org/infra/go/ds/testutil"
	"go.skia.org/infra/go/eventbus"
	ingestion_mocks "go.skia.org/infra/go/ingestion/mocks"
	"go.skia.org/infra/go/testutils"
	"go.skia.org/infra/go/testutils/unittest"
	"go.skia.org/infra/go/util"
	"go.skia.org/infra/go/vcsinfo"
	"go.skia.org/infra/go/vcsinfo/mocks"
	"go.skia.org/infra/golden/go/tryjobstore"
	"go.skia.org/infra/golden/go/tryjobstore/ds_tryjobstore"
)

const (
	// directory with the test data.
	TEST_DATA_DIR = "./testdata"

	// name of the input file containing test data.
	TRYJOB_INGESTION_FILE = TEST_DATA_DIR + "/tryjob-dm.json"
)

// Tests the processor in conjunction with the vcs.
// TODO(kjlubick): make this test use mocks for tryjobstore and VCS.
func TestTryjobGoldProcessor(t *testing.T) {
	unittest.LargeTest(t)

	cleanup := testutil.InitDatastore(t,
		ds.ISSUE,
		ds.TRYJOB,
		ds.TRYJOB_RESULT)
	defer cleanup()

	issueUpdated, err := time.Parse("2006-01-02 15:04:05 MST", "2017-12-07 14:54:05 EST")
	assert.NoError(t, err)

	testIssue := &tryjobstore.Issue{
		ID:      81300,
		Subject: "[infra] Move commands from isolates to gen_tasks.go",
		Owner:   "someone@example.com",
		Status:  "MERGED",
		Updated: issueUpdated,
		PatchsetDetails: []*tryjobstore.PatchsetDetail{
			{
				ID: 9,
			},
		},
	}
	testTryjob := &tryjobstore.Tryjob{
		BuildBucketID: 8960860541739306896,
		IssueID:       81300,
		PatchsetID:    9,
		Builder:       "Test-iOS-Clang-iPhone7-GPU-GT7600-arm64-Debug-All",
		Status:        tryjobstore.TRYJOB_COMPLETE,
		Updated:       time.Unix(1512655545, 180550*int64(time.Microsecond)),
	}

	noUploadTryjob := &tryjobstore.Tryjob{
		BuildBucketID: 8960860541739406896,
		IssueID:       81300,
		PatchsetID:    9,
		Builder:       "Test-iOS-Clang-iPhone7-GPU-GT7600-arm64-Debug-ASAN",
		Status:        tryjobstore.TRYJOB_COMPLETE,
		Updated:       time.Unix(1512655545, 180550*int64(time.Microsecond)),
	}

	// Set up the TryjobStore.
	eventBus := eventbus.New()
	tryjobStore, err := ds_tryjobstore.New(ds.DS, eventBus)
	assert.NoError(t, err)

	// Map the path of the file to its content
	cfgFile := "infra/bots/cfg.json"
	fileContentMap := map[string]string{
		cfgFile: `{
			"gs_bucket_gm": "skia-infra-gm",
			"gs_bucket_nano": "skia-perf",
			"gs_bucket_coverage": "skia-coverage",
			"gs_bucket_calm": "skia-calmbench",
			"pool": "Skia",
			"no_upload": [
				"ASAN",
				"Coverage",
				"MSAN",
				"TSAN",
				"UBSAN",
				"Valgrind",
				"AbandonGpuContext",
				"SKQP"
			]
		}`,
	}
	mockVCS := mocks.DeprecatedMockVCS([]*vcsinfo.LongCommit{}, nil, fileContentMap)

	// Make sure the issue is removed.
	assert.NoError(t, tryjobStore.DeleteIssue(testIssue.ID))
	mockedIBF := &mockIBF{
		issue:       testIssue,
		tryjob:      testTryjob,
		tryjobStore: tryjobStore,
	}

	// Instantiate the processor and add a channel to capture the callback.
	callbackCh := make(chan interface{}, 20)
	processor := &goldTryjobProcessor{
		buildIssueSync: mockedIBF,
		tryjobStore:    tryjobStore,
		vcs:            mockVCS,
		cfgFile:        cfgFile,
		syncMonitor:    util.NewCondMonitor(1),
	}
	eventBus.SubscribeAsync(tryjobstore.EV_TRYJOB_UPDATED, func(data interface{}) {
		processor.tryjobUpdatedHandler(data)
		callbackCh <- data
	})

	// Call process for the input file.
	fsResult, err := ingestion_mocks.MockResultFileLocationFromFile(TRYJOB_INGESTION_FILE)
	assert.NoError(t, err)
	assert.NoError(t, processor.Process(context.Background(), fsResult))

	foundIssue, err := tryjobStore.GetIssue(testIssue.ID, false)
	assert.NoError(t, err)
	foundIssue.Updated = testIssue.Updated
	assert.Equal(t, testIssue, foundIssue)

	foundTryjob, err := tryjobStore.GetTryjob(testIssue.ID, testTryjob.BuildBucketID)
	assert.NoError(t, err)

	// At this point the tryjob should be marked ingested.
	testTryjob.Status = tryjobstore.TRYJOB_INGESTED
	foundTryjob.Key = nil
	assert.Equal(t, testTryjob, foundTryjob)

	// Write a tryjob result that doesn't upload and make sure the status is
	// updated correct upon completion.
	assert.NoError(t, tryjobStore.UpdateTryjob(0, noUploadTryjob, nil))

	calledBack := false
	eventsFound := 0
	assert.NoError(t, testutils.EventuallyConsistent(10*time.Second, func() error {
		data := <-callbackCh
		tryjob := data.(*tryjobstore.Tryjob)
		calledBack = calledBack || (tryjob.Builder == noUploadTryjob.Builder)

		// At this point we should have gathered 5 events.
		// Two for each ingested tryjob and one for the UpdateTryjob call above.
		eventsFound++
		if eventsFound == 5 {
			return nil
		}
		return testutils.TryAgainErr
	}))

	assert.True(t, calledBack)
	assert.Equal(t, 0, len(callbackCh))

	// Closing the channel in an earlier version caused a data race. Close it
	// to make sure that is resolved.
	close(callbackCh)

	foundTryjob, err = tryjobStore.GetTryjob(testIssue.ID, noUploadTryjob.BuildBucketID)
	assert.NoError(t, err)
	assert.Equal(t, tryjobstore.TRYJOB_INGESTED, foundTryjob.Status)
}

type mockIBF struct {
	issue       *tryjobstore.Issue
	tryjob      *tryjobstore.Tryjob
	tryjobStore tryjobstore.TryjobStore
}

func (m *mockIBF) SyncIssueTryjob(issueID, buildBucketID int64) (*tryjobstore.Issue, *tryjobstore.Tryjob, error) {
	if issueID != m.issue.ID {
		return nil, nil, fmt.Errorf("Unknown issued.")
	}

	if buildBucketID != m.tryjob.BuildBucketID {
		return nil, nil, fmt.Errorf("Unknown buildbucket id.")
	}

	// Make sure the issue tryjob are in the store.
	if err := m.tryjobStore.UpdateIssue(m.issue, nil); err != nil {
		return nil, nil, err
	}

	if err := m.tryjobStore.UpdateTryjob(0, m.tryjob, nil); err != nil {
		return nil, nil, err
	}

	return m.issue, m.tryjob, nil
}
