Storing Expectations in Firestore
=================================

Gold Expectations are essentially a map of (Grouping, Digest) to Label where Grouping is
currently TestName (but could be a combination of TestName + ColorSpace or something
more complex), Digest is the md5 hash of an image's content, and Label is Positive, Negative,
or Untriaged (default).

There is the idea of the master expectations, which is the Expectations belonging to the
git branch "master". Additionally, there can be smaller CL expectations that belong
to a ChangeList (CL) and stay in a separate partition from the master expectations until the
CL lands. These CL expectations are the "delta" that would be applied to the master expectations.

We'd like to be able to do the following:

  - Store and retrieve Expectations (both master expectations and CL expectations).
  - Update the Label for a (Grouping, Digest).
  - Keep an audit record of what user updated the Label for a given (Grouping, Digest).
  - Undo a previous change.
  - Support Gerrit CLs and GitHub PullRequests (PRs)

Schema
------

For a background on Firestore and its data model, see
<https://firebase.google.com/docs/firestore/data-model>.

Like all other projects, we will use the firestore.NewClient to create a top level
"gold" collection with a parent document for this instance (e.g. "skia-prod", "flutter", etc).
Underneath that instance-specific document, we have a collection of partition documents (e.g.
"master", "gerrit_1234"). These partition documents have three collections: *entries*,
*triage_records*, and *triage_changes*. This arrangement allows us to more easily keep data
separate between the partitions. As an example, we don't have to have a
`Where(partition, "==", "foo")` clause on all of our queries.

This means our structure looks like (using the instance-specific-document as the root):
```
/partitions/master/entries/my_test|12345
/partitions/master/entries/my_test|abcde
/partitions/master/entries/my_other_test|987654
...
/partitions/master/records/000001
/partitions/master/records/000002
...
/partitions/master/changes/000001
/partitions/master/changes/000002
...
/partitions/gerrit_12345/entries/my_test|12345
/partitions/gerrit_12345/entries/my_test|zxyxx
...
```

In the *entries* collection, we will store many `expectationEntry` documents with
the following schema:

	Grouping       string    # This is currently the TestName
	Digest         string
	Updated        time.Time
	LastUsed       time.Time # used to clean up unused expectations.
	Ranges         []triageRange # stores Label in a future-proof way. See "Future Changes" below.
	NeedsGC        boolean # used to cleanup unused expectations.

The `expectationEntry` will have an ID of `grouping|digest`, allowing updates to not require
doing a search with a double `Where()`. For example, an id might look like:
`"my_very_interesting_test|2c678c17946f9e028cc98ca02176b7f6"`

The *triage_records* collection will have `triageRecords` documents, which use an autogenerated ID:

	UserName     string
	TS           time.Time
	Committed    bool      # if writing has completed (e.g. large triage)
	Changes      int       # how many records match in triage_changes collection

The *triage_changes* collection will have `expectationChange` documents:

	RecordID       string # From the triage_records collection
	Grouping       string
	Digest         string
	LabelBefore    int
	AffectedRange  triageRange

We split the triage data into two collections to account for the fact that bulk triages can
sometimes be for thousands of entries, which would surpass the 1Mb Firestore limit per document.

Indexing
--------
In addition to the default single-field indexes, we need the following composite index:

Collection ID              | Fields
------------------------------------------------------------------
triage_records             | committed: ASC ts: DESC

Usage
-----

As an option, the expectations can be kept in RAM (and up-to-date) through use of Firestore's
query snapshots. This is done with the Initialize function.

For performance, we shard fetching the expectations based on digest, since that data is essentially
random and evenly distributed.

When the tryjob monitor notes that a CL has landed, it can fetch the given CL expectations and
apply them to the master branch.

To undo, we can query the original change by id (from the `triage_records` collection) and update
the affected range with the LabelBefore if the current set of ranges exactly contains the
AffectedRange.

To cleanup old expectations, a process calls `UpdateLastUsed`, which updates the `last_used` field.
That same process then calls `MarkUnusedEntriesForGC`, which sets the `NeedsGC` field to true for
all `expectationEntry` documents which have a `last_used` and `updated` field before the given time.
These are then deleted with a call to `GarbageCollect`.

Future Changes
--------------

As of Mar 2020, there is consideration for Chrome's Web Tests to use Gold. One requirement is that
Gold's expectations apply only sometimes, not always. Thus, we store ranges, which would allow that
to happen. For now, the ranges are just 1 range from 0 to MaxInt, so effectively rangeless, as
it was before.