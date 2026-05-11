package connect

import (
	"context"

	"connectrpc.com/connect"

	"github.com/pitabwire/frame/datastore/pool"
)

// NewTenancyTxInterceptor returns a Connect interceptor that runs every
// RPC inside a request-scoped tenancy transaction. The interceptor
// invokes pool.WithRequestTx, which:
//
//  1. Opens a transaction on a pooled connection.
//  2. Publishes app.tenant_id (single value) and app.partition_id
//     (comma-separated list — one principal may legitimately span
//     multiple partitions) from the auth claims via set_config(..., true)
//     so the values are SET LOCAL and revert when the transaction
//     commits / rolls back.
//  3. Binds the transaction to the request context so downstream
//     pool.DB(ctx, _) calls return the same tx, end-to-end.
//
// Combined with the Row-Level Security policies installed automatically
// by pool.Migrate on every data.BaseModel-embedding table, this means
// the application's repository code never references tenant_id or
// partition_id directly — frame and Postgres enforce isolation between
// them.
//
// Register after the authentication interceptor so the auth claims are
// available when WithRequestTx reads them. The auto-applied
// scopes.TenancyPartition still runs for trivial GORM-builder paths
// where it can prefix the table alias correctly; this interceptor is
// what makes naive Raw SQL and multi-table joins transparent.
//
// Streaming handlers (server-streaming RPCs that send batches via a
// workerpool) hold the transaction open for the duration of the
// stream. That is intentional: every batch reads through the same
// session-variable scope. Pure-read streams are safe; mutate-then-
// stream patterns inherit the transaction's commit semantics.
func NewTenancyTxInterceptor(dbPool pool.Pool) connect.Interceptor {
	return &tenancyTxInterceptor{pool: dbPool}
}

type tenancyTxInterceptor struct {
	pool pool.Pool
}

func (t *tenancyTxInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		var resp connect.AnyResponse
		err := t.pool.WithRequestTx(ctx, func(rCtx context.Context) error {
			r, callErr := next(rCtx, req)
			if callErr != nil {
				return callErr
			}
			resp = r
			return nil
		})
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

// WrapStreamingClient is a server-only interceptor; client streams
// pass through untouched.
func (t *tenancyTxInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (t *tenancyTxInterceptor) WrapStreamingHandler(
	next connect.StreamingHandlerFunc,
) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return t.pool.WithRequestTx(ctx, func(rCtx context.Context) error {
			return next(rCtx, conn)
		})
	}
}
