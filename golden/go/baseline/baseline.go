// package baseline contains functions to gather the current baseline and
// write them to GCS.
package baseline

import (
	"fmt"

	"go.skia.org/infra/go/skerr"
	"go.skia.org/infra/go/tiling"
	"go.skia.org/infra/go/util"
	"go.skia.org/infra/golden/go/tryjobstore"
	"go.skia.org/infra/golden/go/types"
)

// md5SumEmptyExp is the MD5 sum of an empty expectation.
// it is initialized in this file's init().
var md5SumEmptyExp = ""

func init() {
	var err error
	md5SumEmptyExp, err = util.MD5Sum(types.Expectations{})
	if err != nil {
		panic(fmt.Sprintf("Could not get the MD5 sum of an empty expectation: %s", err))
	}
}

// GetBaselineForIssue returns the baseline for the given issue. This baseline
// contains all triaged digests that are not in the master tile.
// Note: Total and Filled are not relevant for an issue baseline since
// the concept of traces doesn't really make sense for a single commit.
func GetBaselineForIssue(issueID int64, tryjobs []*tryjobstore.Tryjob, tryjobResults [][]*tryjobstore.TryjobResult, exp types.Expectations, commits []*tiling.Commit) (*Baseline, error) {
	startCommit := commits[len(commits)-1]
	endCommit := commits[len(commits)-1]

	b := types.Expectations{}
	for idx, tryjob := range tryjobs {
		for _, result := range tryjobResults[idx] {
			if result.Digest != types.MISSING_DIGEST && exp.Classification(result.TestName, result.Digest) == types.POSITIVE {
				b.AddDigest(result.TestName, result.Digest, types.POSITIVE)
			}

			_, c := tiling.FindCommit(commits, tryjob.MasterCommit)
			startCommit = minCommit(startCommit, c)
			endCommit = maxCommit(endCommit, c)
		}
	}

	md5Sum, err := util.MD5Sum(b)
	if err != nil {
		return nil, skerr.Fmt("Error calculating MD5 sum: %s", err)
	}

	// TODO(stephana): Review whether StartCommit and EndCommit are useful here.

	ret := &Baseline{
		StartCommit:  startCommit,
		EndCommit:    endCommit,
		Expectations: b,
		Issue:        issueID,
		MD5:          md5Sum,
	}
	return ret, nil
}

// minCommit returns newCommit if it appears before current (or current is nil).
func minCommit(current *tiling.Commit, newCommit *tiling.Commit) *tiling.Commit {
	if current == nil || newCommit == nil || newCommit.CommitTime < current.CommitTime {
		return newCommit
	}
	return current
}

// maxCommit returns newCommit if it appears after current (or current is nil).
func maxCommit(current *tiling.Commit, newCommit *tiling.Commit) *tiling.Commit {
	if current == nil || newCommit == nil || newCommit.CommitTime > current.CommitTime {
		return newCommit
	}
	return current
}
