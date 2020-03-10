package repo_manager

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.skia.org/infra/go/exec"
	"go.skia.org/infra/go/gerrit"
	"go.skia.org/infra/go/git"
	git_testutils "go.skia.org/infra/go/git/testutils"
	gitiles_testutils "go.skia.org/infra/go/gitiles/testutils"
	"go.skia.org/infra/go/mockhttpclient"
	"go.skia.org/infra/go/testutils"
	"go.skia.org/infra/go/testutils/unittest"
	"go.skia.org/infra/go/util"
)

func fuchsiaAndroidCfg(t *testing.T) *FuchsiaSDKAndroidRepoManagerConfig {
	return &FuchsiaSDKAndroidRepoManagerConfig{
		FuchsiaSDKRepoManagerConfig: FuchsiaSDKRepoManagerConfig{
			NoCheckoutRepoManagerConfig: NoCheckoutRepoManagerConfig{
				CommonRepoManagerConfig: CommonRepoManagerConfig{
					ChildBranch:  masterBranchTmpl(t),
					ChildPath:    "external/fuchsia_sdk",
					ParentBranch: masterBranchTmpl(t),
				},
			},
		},
		GenSdkBpRepo:   "TODO",
		GenSdkBpBranch: "master",
	}
}

func TestFuchsiaSDKAndroidConfig(t *testing.T) {
	unittest.SmallTest(t)

	cfg := fuchsiaAndroidCfg(t)
	cfg.ParentRepo = "todo.git"
	require.NoError(t, cfg.Validate())
}

func setupFuchsiaSDKAndroid(t *testing.T) (context.Context, *fuchsiaSDKAndroidRepoManager, *mockhttpclient.URLMock, *gitiles_testutils.MockRepo, *git_testutils.GitBuilder, func()) {
	wd, err := ioutil.TempDir("", "")
	require.NoError(t, err)

	cfg := fuchsiaAndroidCfg(t)

	// Mock out git commands.
	mockRun := exec.CommandCollector{}
	mockRun.SetDelegateRun(func(ctx context.Context, cmd *exec.Command) error {
		if strings.Contains(cmd.Name, "git") && util.In("push", cmd.Args) {
			// Don't run "git push".
			return nil
		} else if util.In(FUCHSIA_SDK_ANDROID_GEN_SCRIPT, cmd.Args) {
			// Write a dummy file to imitate the SDK generation.
			sdkPath := cmd.Args[len(cmd.Args)-1]
			testutils.WriteFile(t, filepath.Join(sdkPath, "bogus"), "bogus")
			return nil
		} else {
			return exec.DefaultRun(ctx, cmd)
		}
	})
	ctx := exec.NewContext(context.Background(), mockRun.Run)

	// Create repos.
	parent := git_testutils.GitInit(t, ctx)
	parent.Add(ctx, FUCHSIA_SDK_ANDROID_VERSION_FILE, fuchsiaSDKRevBase)
	parent.Commit(ctx)
	cfg.ParentRepo = parent.RepoUrl()

	// This is not technically correct, but the call into gen_sdk_bp is
	// mocked and we have to check out something.
	cfg.GenSdkBpRepo = parent.RepoUrl()

	urlmock := mockhttpclient.NewURLMock()
	mockParent := gitiles_testutils.NewMockRepo(t, parent.RepoUrl(), git.GitDir(parent.Dir()), urlmock)

	gUrl := "https://fake-skia-review.googlesource.com"
	serialized, err := json.Marshal(&gerrit.AccountDetails{
		AccountId: 101,
		Name:      mockUser,
		Email:     mockUser,
		UserName:  mockUser,
	})
	require.NoError(t, err)
	serialized = append([]byte("abcd\n"), serialized...)
	urlmock.MockOnce(gUrl+"/a/accounts/self/detail", mockhttpclient.MockGetDialogue(serialized))
	g, err := gerrit.NewGerritWithConfig(gerrit.CONFIG_ANDROID, gUrl, urlmock.Client())
	require.NoError(t, err)

	// Initial update, everything up-to-date.
	mockParent.MockGetCommit(ctx, "master")
	parentMaster, err := git.GitDir(parent.Dir()).RevParse(ctx, "HEAD")
	require.NoError(t, err)
	mockParent.MockReadFile(ctx, FUCHSIA_SDK_ANDROID_VERSION_FILE, parentMaster)
	mockGetLatestSDK(urlmock, FUCHSIA_SDK_GS_LATEST_PATH_LINUX, FUCHSIA_SDK_GS_LATEST_PATH_MAC, fuchsiaSDKRevBase, "mac-base")

	rm, err := NewFuchsiaSDKAndroidRepoManager(ctx, cfg, setupRegistry(t), wd, g, "fake.server.com", urlmock.Client(), androidGerrit(t, g), false)
	require.NoError(t, err)

	cleanup := func() {
		testutils.RemoveAll(t, wd)
		parent.Cleanup()
	}

	return ctx, rm.(*fuchsiaSDKAndroidRepoManager), urlmock, mockParent, parent, cleanup
}

func TestFuchsiaSDKAndroidRepoManager(t *testing.T) {
	unittest.LargeTest(t)

	ctx, rm, urlmock, mockParent, parent, cleanup := setupFuchsiaSDKAndroid(t)
	defer cleanup()

	lastRollRev, tipRev, notRolledRevs, err := rm.Update(ctx)
	require.NoError(t, err)
	require.Equal(t, fuchsiaSDKRevBase, lastRollRev.Id)
	require.Equal(t, fuchsiaSDKRevBase, tipRev.Id)
	prev, err := rm.GetRevision(ctx, fuchsiaSDKRevPrev)
	require.NoError(t, err)
	require.Equal(t, fuchsiaSDKRevPrev, prev.Id)
	base, err := rm.GetRevision(ctx, fuchsiaSDKRevBase)
	require.NoError(t, err)
	require.Equal(t, fuchsiaSDKRevBase, base.Id)
	next, err := rm.GetRevision(ctx, fuchsiaSDKRevNext)
	require.NoError(t, err)
	require.Equal(t, fuchsiaSDKRevNext, next.Id)
	require.Empty(t, rm.preUploadSteps)
	require.Equal(t, 0, len(notRolledRevs))

	// There's a new version.
	mockParent.MockGetCommit(ctx, "master")
	parentMaster, err := git.GitDir(parent.Dir()).RevParse(ctx, "HEAD")
	require.NoError(t, err)
	mockParent.MockReadFile(ctx, FUCHSIA_SDK_ANDROID_VERSION_FILE, parentMaster)
	mockGetLatestSDK(urlmock, FUCHSIA_SDK_GS_LATEST_PATH_LINUX, FUCHSIA_SDK_GS_LATEST_PATH_MAC, fuchsiaSDKRevNext, "mac-next")

	lastRollRev, tipRev, notRolledRevs, err = rm.Update(ctx)
	require.NoError(t, err)
	require.Equal(t, fuchsiaSDKRevBase, lastRollRev.Id)
	require.Equal(t, fuchsiaSDKRevNext, tipRev.Id)
	require.Equal(t, 1, len(notRolledRevs))
	require.Equal(t, fuchsiaSDKRevNext, notRolledRevs[0].Id)

	// Upload a CL.

	// Create a dummy commit-msg hook.
	changeId := "123"
	hookFile := filepath.Join(rm.parentRepo.Dir(), ".git", "hooks", "commit-msg")
	testutils.WriteFile(t, hookFile, fmt.Sprintf(`#!/bin/sh
git interpret-trailers --trailer "Change-Id: %s" > $1
`, changeId))

	// Mock the request to get the CL.
	ci := gerrit.ChangeInfo{
		ChangeId: changeId,
		Id:       changeId,
		Issue:    123,
		Revisions: map[string]*gerrit.Revision{
			"ps1": {
				ID:     "ps1",
				Number: 1,
			},
		},
	}
	respBody, err := json.Marshal(ci)
	require.NoError(t, err)
	respBody = append([]byte(")]}'\n"), respBody...)
	urlmock.MockOnce("https://fake-skia-review.googlesource.com/a/changes/123/detail?o=ALL_REVISIONS", mockhttpclient.MockGetDialogue(respBody))

	// Mock the request to set the CQ.
	reqBody := []byte(`{"labels":{"Autosubmit":1,"Code-Review":2,"Presubmit-Ready":1},"message":"","reviewers":[{"reviewer":"reviewer@chromium.org"}]}`)
	urlmock.MockOnce("https://fake-skia-review.googlesource.com/a/changes/123/revisions/ps1/review", mockhttpclient.MockPostDialogue("application/json", reqBody, []byte("")))

	issue, err := rm.CreateNewRoll(ctx, lastRollRev, tipRev, notRolledRevs, emails, cqExtraTrybots, false)
	require.NoError(t, err)
	require.Equal(t, ci.Issue, issue)
}

func TestFuchsiaSDKAndroidConfigValidation(t *testing.T) {
	unittest.SmallTest(t)

	cfg := fuchsiaAndroidCfg(t)
	cfg.ParentRepo = "dummy" // Not supplied above.
	require.NoError(t, cfg.Validate())

	cfg.GenSdkBpRepo = ""
	require.EqualError(t, cfg.Validate(), "GenSdkBpRepo is required.")

	// The remaining fields come from the nested Configs, so exclude them
	// and verify that we fail validation.
	cfg = fuchsiaAndroidCfg(t)
	cfg.FuchsiaSDKRepoManagerConfig = FuchsiaSDKRepoManagerConfig{}
	require.Error(t, cfg.Validate())
}
