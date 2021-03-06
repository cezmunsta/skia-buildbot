package tracestore

import (
	"context"
	"time"

	"go.skia.org/infra/go/paramtools"
	"go.skia.org/infra/go/query"
	"go.skia.org/infra/perf/go/types"
)

// TraceStore is the interface that all backends that store traces must
// implement. It is used by dfbuilder to build DataFrames and by the perf-tool
// to perform some common maintenance tasks.
type TraceStore interface {
	// CommitNumberOfTileStart returns the types.CommitNumber at the beginning of the
	// given tile.
	CommitNumberOfTileStart(commitNumber types.CommitNumber) types.CommitNumber

	// CountIndices returns the number of index rows that exist for the given
	// tileKey.
	CountIndices(ctx context.Context, tileNumber types.TileNumber) (int64, error)

	// GetLatestTile returns the latest, i.e. the newest tile.
	GetLatestTile() (types.TileNumber, error)

	// GetOrderedParamSet returns the OPS for the given tile.
	GetOrderedParamSet(ctx context.Context, tileNumber types.TileNumber) (*paramtools.OrderedParamSet, error)

	// GetSource returns the full URL of the file that contained the point at
	// 'index' of trace 'traceId'.
	GetSource(ctx context.Context, commitNumber types.CommitNumber, traceId string) (string, error)

	// OffsetFromIndex returns the offset from within a Tile that a commit sits.
	OffsetFromIndex(commitNumber types.CommitNumber) int32

	// QueryTracesByIndex returns a map of trace keys to a slice of floats for
	// all traces that match the given query.
	QueryTracesByIndex(ctx context.Context, tileNumber types.TileNumber, q *query.Query) (types.TraceSet, error)

	// QueryTracesIDOnlyByIndex returns a stream of ParamSets that match the
	// given query.
	QueryTracesIDOnlyByIndex(ctx context.Context, tileNumber types.TileNumber, q *query.Query) (<-chan paramtools.Params, error)

	// ReadTraces loads the traces for the given trace keys.
	ReadTraces(tileNumber types.TileNumber, keys []string) (types.TraceSet, error)

	// TileNumber returns the types.TileNumber that the commit is stored in.
	TileNumber(commitNumber types.CommitNumber) types.TileNumber

	// TileSize returns the size of a Tile.
	TileSize() int32

	// TraceCount returns the number of traces in a tile.
	TraceCount(ctx context.Context, tileNumber types.TileNumber) (int64, error)

	// WriteIndices recalculates the full index for the given tile and writes it
	// back to the underlying datastore.
	WriteIndices(ctx context.Context, tileNumber types.TileNumber) error

	// WriteTraces writes the given values into the store.
	//
	// params is a slice of Params, where each one represents a single trace.
	// values are the values to write, for each trace in params, at the offset
	// given in types.CommitNumber. paramset is the ParamSet of all the params
	// to be written. source is the filename where the data came from. timestamp
	// is the timestamp when the data was generated.
	//
	// Note that 'params' and 'values' are parallel slices and thus need to
	// match.
	WriteTraces(commitNumber types.CommitNumber, params []paramtools.Params, values []float32, paramset paramtools.ParamSet, source string, timestamp time.Time) error
}
