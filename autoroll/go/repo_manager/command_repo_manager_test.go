// +build linux

package repo_manager

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"go.skia.org/infra/autoroll/go/repo_manager/common/git_common"
	"go.skia.org/infra/go/exec"
	"go.skia.org/infra/go/gerrit"
	"go.skia.org/infra/go/git"
	git_testutils "go.skia.org/infra/go/git/testutils"
	"go.skia.org/infra/go/mockhttpclient"
	"go.skia.org/infra/go/skerr"
	"go.skia.org/infra/go/sklog"
	"go.skia.org/infra/go/testutils"
	"go.skia.org/infra/go/testutils/unittest"
	"go.skia.org/infra/go/util"
	"go.skia.org/infra/go/vcsinfo"
)

func commandsEqual(a, b *exec.Command) bool {
	if a.Name != b.Name {
		sklog.Errorf("Name")
		return false
	}
	if !util.SSliceEqual(a.Args, b.Args) {
		sklog.Errorf("Args: %v %v", a.Args, b.Args)
		return false
	}
	if a.Dir != b.Dir {
		sklog.Errorf("Dir")
		return false
	}
	if a.InheritEnv != b.InheritEnv {
		sklog.Errorf("InheritEnv")
		return false
	}
	if a.InheritPath != b.InheritPath {
		sklog.Errorf("InheritPath")
		return false
	}
	if !util.SSliceEqual(a.Env, b.Env) {
		sklog.Errorf("Env")
		return false
	}
	return (a.Name == b.Name &&
		util.SSliceEqual(a.Args, b.Args) &&
		a.Dir == b.Dir &&
		a.InheritEnv == b.InheritEnv &&
		a.InheritPath == b.InheritPath &&
		util.SSliceEqual(a.Env, b.Env))
}

func TestCommandRepoManager(t *testing.T) {
	unittest.SmallTest(t)

	const tipRev0 = "tipRev0"
	const pinnedRev0 = "pinnedRev0"

	// Setup.
	ctx := context.Background()
	tmp, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer testutils.RemoveAll(t, tmp)
	urlmock := mockhttpclient.NewURLMock()
	g := setupFakeGerrit(t, depsCfg(t).Gerrit, urlmock)
	parent := git_testutils.GitInit(t, ctx)
	parent.Add(ctx, "version", "pinnedRev0")
	parent.Commit(ctx)
	baseDir := filepath.Join(tmp, filepath.Base(parent.RepoUrl()))

	// Commands used by this CommandRepoManager.
	vars := &CommandTmplVars{
		RollingFrom: pinnedRev0,
		RollingTo:   tipRev0,
	}
	getTipRev := &CommandConfig{
		Command: []string{"echo", tipRev0},
		Dir:     ".",
		Env: map[string]string{
			"key": "val",
		},
	}
	getTipRevCmd, err := makeCommand(getTipRev, baseDir, vars)
	require.NoError(t, err)
	getTipRevCount := 0

	getPinnedRev := &CommandConfig{
		Command: []string{"cat", "version"},
		Dir:     ".",
		Env: map[string]string{
			"key2": "val2",
		},
	}
	getPinnedRevCmd, err := makeCommand(getPinnedRev, baseDir, vars)
	require.NoError(t, err)
	getPinnedRevCount := 0

	setPinnedRev := &CommandConfig{
		Command: []string{"bash", "-c", "echo \"{{.RollingTo}}\" > version"},
		Dir:     ".",
		Env: map[string]string{
			"key3": "val3",
		},
	}
	setPinnedRevCmd, err := makeCommand(setPinnedRev, baseDir, vars)
	require.NoError(t, err)
	setPinnedRevCount := 0

	cfg := CommandRepoManagerConfig{
		GitCheckoutConfig: git_common.GitCheckoutConfig{
			Branch:  masterBranchTmpl(t),
			RepoURL: parent.RepoUrl(),
		},
		GetTipRev:    getTipRev,
		GetPinnedRev: getPinnedRev,
		SetPinnedRev: setPinnedRev,
	}

	// Mock all commands. If the command is one of the three special commands
	// for this repo manager, verify that it matches expectations.
	lastUpload := new(vcsinfo.LongCommit)
	mockRun := &exec.CommandCollector{}
	mockRun.SetDelegateRun(func(ctx context.Context, cmd *exec.Command) error {
		sklog.Errorf("MockRun: %s %s", cmd.Name, strings.Join(cmd.Args, " "))
		if commandsEqual(cmd, getTipRevCmd) {
			getTipRevCount++
		}
		sklog.Errorf("1: %s %s", cmd.Name, strings.Join(cmd.Args, " "))
		if commandsEqual(cmd, getPinnedRevCmd) {
			getPinnedRevCount++
		}
		sklog.Errorf("2: %s %s", cmd.Name, strings.Join(cmd.Args, " "))
		if commandsEqual(cmd, setPinnedRevCmd) {
			setPinnedRevCount++
		}
		sklog.Errorf("3: %s %s", cmd.Name, strings.Join(cmd.Args, " "))
		sklog.Errorf(spew.Sdump(setPinnedRevCmd))
		sklog.Errorf(spew.Sdump(cmd))

		// Don't perform "git push".
		if strings.Contains(cmd.Name, "git") && cmd.Args[0] == "push" {
			d, err := git.GitDir(cmd.Dir).Details(ctx, "HEAD")
			if err != nil {
				return skerr.Wrap(err)
			}
			*lastUpload = *d
			return nil
		}

		return exec.DefaultRun(ctx, cmd)
	})
	ctx = exec.NewContext(ctx, mockRun.Run)

	// Create the repo manager.
	rm, err := NewCommandRepoManager(ctx, cfg, setupRegistry(t), tmp, g, "fake.server.com", gerritCR(t, g))
	require.NoError(t, err)
	require.Equal(t, 0, getTipRevCount)
	require.Equal(t, 0, getPinnedRevCount)
	require.Equal(t, 0, setPinnedRevCount)

	// Update.
	lastRollRev, tipRev, notRolledRevs, err := rm.Update(ctx)
	require.NoError(t, err)
	require.NotNil(t, lastRollRev)
	require.NotNil(t, tipRev)
	require.NotNil(t, notRolledRevs)
	require.Equal(t, 1, getTipRevCount)
	require.Equal(t, 1, getPinnedRevCount)
	require.Equal(t, 0, setPinnedRevCount)
	require.Equal(t, pinnedRev0, lastRollRev.Id)
	require.Equal(t, tipRev0, tipRev.Id)
	require.Len(t, notRolledRevs, 1)
	require.Equal(t, tipRev0, notRolledRevs[0].Id)

	// Mock the request to load the change.
	// TODO(borenet): Refactor Gerrit mocks.
	ci := gerrit.ChangeInfo{
		ChangeId: "123",
		Id:       "123",
		Issue:    123,
		Revisions: map[string]*gerrit.Revision{
			"ps1": {
				ID:     "ps1",
				Number: 1,
			},
		},
		WorkInProgress: true,
	}
	respBody, err := json.Marshal(ci)
	require.NoError(t, err)
	respBody = append([]byte(")]}'\n"), respBody...)
	urlmock.MockOnce("https://fake-skia-review.googlesource.com/a/changes/123/detail?o=ALL_REVISIONS", mockhttpclient.MockGetDialogue(respBody))

	// Mock the request to set the change as read for review. This is only
	// done if ChangeInfo.WorkInProgress is true.
	reqBody := []byte(`{}`)
	urlmock.MockOnce("https://fake-skia-review.googlesource.com/a/changes/123/ready", mockhttpclient.MockPostDialogue("application/json", reqBody, []byte("")))

	// Mock the request to set the CQ.
	reqBody = []byte(`{"labels":{"Code-Review":1,"Commit-Queue":2},"message":"","reviewers":[{"reviewer":"reviewer@google.com"}]}`)
	urlmock.MockOnce("https://fake-skia-review.googlesource.com/a/changes/123/revisions/ps1/review", mockhttpclient.MockPostDialogue("application/json", reqBody, []byte("")))

	issue, err := rm.CreateNewRoll(ctx, lastRollRev, tipRev, notRolledRevs, []string{"reviewer@google.com"}, false, "fake-commit-msg")
	require.NoError(t, err)
	require.NotEqual(t, 0, issue)
}
