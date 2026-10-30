package tasks

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// CustomerAgentPoolEnv is the queue env var that names a pool of the queue to
// own every customer status of its workflow in place of the customer. A task
// reads it when it enters such a status; its visit keeps the pool.
const CustomerAgentPoolEnv = "CUSTOMER_AGENT_POOL"

// taskStatus is currentStatus for the task's current status, owned by the pool
// its open visit recorded in place of the customer.
func taskStatus(manifest workflowimage.Manifest, task Task) (workflowfile.Status, bool) {
	status, ok := currentStatus(manifest, task.WorkflowStatus)
	return withOwnerPool(status, task.workflowOwnerPool), ok
}

// withOwnerPool returns a customer status owned by pool instead, when pool is
// set; any other status unchanged.
func withOwnerPool(status workflowfile.Status, pool string) workflowfile.Status {
	if pool != "" && !status.Terminal && status.Owner.Kind == workflowfile.OwnerCustomer {
		status.Owner = workflowfile.Owner{Kind: workflowfile.OwnerPool, Pool: pool}
	}
	return status
}

// customerAgentPoolTx returns the queue's CUSTOMER_AGENT_POOL env var, "" when
// it has none. A secret of that name is never read.
func customerAgentPoolTx(ctx context.Context, q queryer, queue string) (string, error) {
	var pool string
	err := q.QueryRowContext(ctx, `
		SELECT value FROM task_queue_secrets WHERE queue_prefix = ? AND key = ? AND secret = 0`,
		queue, CustomerAgentPoolEnv).Scan(&pool)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return pool, err
}

// requireCustomerAgentPoolTx rejects pool as the CUSTOMER_AGENT_POOL of a
// queue bound to a workflow when the queue has no such pool or it has no
// member. An unbound queue is checked when it binds.
func requireCustomerAgentPoolTx(ctx context.Context, q queryer, queue, pool string) error {
	if _, bound, err := queueWorkflowTx(ctx, q, queue); err != nil || !bound {
		return err
	}
	missing, err := emptyPools(ctx, q, queue, []string{pool})
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return poolsEmptyError(missing)
	}
	return nil
}

// workflowPoolsTx returns the pools manifest needs in queue: the pools it
// declares and, when it has a customer status, the queue's
// CUSTOMER_AGENT_POOL.
func workflowPoolsTx(ctx context.Context, q queryer, queue string, manifest workflowimage.Manifest) ([]string, error) {
	pools := manifest.Definition.Pools()
	pool, err := customerAgentPoolTx(ctx, q, queue)
	if err != nil || pool == "" || slices.Contains(pools, pool) {
		return pools, err
	}
	for _, status := range manifest.Definition.Statuses {
		if !status.Terminal && status.Owner.Kind == workflowfile.OwnerCustomer {
			return append(slices.Clip(pools), pool), nil
		}
	}
	return pools, nil
}
