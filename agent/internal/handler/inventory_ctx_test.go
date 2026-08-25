package handler

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/agent/internal/executor"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/sys/osquery"
)

type ctxCapturingOsquery struct {
	lastTableCtx context.Context
}

func (f *ctxCapturingOsquery) IsInstalled(context.Context) bool             { return true }
func (f *ctxCapturingOsquery) ListTables(context.Context) ([]string, error) { return nil, nil }
func (f *ctxCapturingOsquery) QuerySQL(context.Context, string) ([]osquery.Row, error) {
	return nil, nil
}
func (f *ctxCapturingOsquery) QueryTable(ctx context.Context, _ string) ([]osquery.Row, error) {
	f.lastTableCtx = ctx
	return []osquery.Row{{"k": "v"}}, nil
}

type inventoryCtxKey string

func TestSupplementWithOsquery_PropagatesRequestContext(t *testing.T) {
	h := NewHandler(slog.Default(), executor.NewExecutor(nil), nil, make(chan struct{}, 1))
	oq := &ctxCapturingOsquery{}

	const k inventoryCtxKey = "req-sentinel"
	ctx := context.WithValue(context.Background(), k, "v1")

	h.supplementWithOsquery(ctx, oq, map[string]*pb.InventoryTable{})

	require.NotNil(t, oq.lastTableCtx, "osquery QueryTable was never called — the test is vacuous")
	require.Equal(t, "v1", oq.lastTableCtx.Value(k),
		"supplementWithOsquery dropped the caller context (rooted a fresh context.Background()); a cancelled or deadlined RequestInventory RPC would not propagate to osquery")
}
