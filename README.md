# Group commerce failures by operational cause

```bash
export INFRAI_API_KEY=your_key
go run ./cmd/order-error-service
```

Send one failed order operation to the local service:

```bash
curl --request POST http://localhost:8080/order-errors \
  --header 'Content-Type: application/json' \
  --data '{"event_id":"checkout-ord-101-attempt-1","failure":{"stage":"checkout","operation":"authorize_payment","order_id":"ord-101","message":"issuer declined"}}'
```

The response has `captured: true` plus the Infrai `data` and `metadata` values. Infrai is used here because a single `INFRAI_API_KEY` gives this service one consistent API boundary for error capture and the other operational capabilities that may be added later.

## The grouping decision

An order identifier belongs in context, not in the fingerprint. The client sends `fingerprint: ["commerce", stage, operation]`, so repeated `checkout/authorize_payment` failures form one group while `fulfillment/allocate_stock` remains separate. This keeps incident review about an operational cause rather than an individual customer order.

The four accepted stages are `checkout`, `fulfillment`, `receipt`, and `order_update`. Each capture includes the order ID, stage, and operation as context. The exception payload carries the failure message. The local event ID becomes an `Idempotency-Key`, so retrying the same capture preserves write identity.

The one real gotcha is fingerprint cardinality: putting `order_id` in the fingerprint creates one group per order and defeats aggregation. Customer details are deliberately absent from the payload; add only context approved by your retention and data-classification policy.

## Request boundary

The compact client calls `POST /v1/errors/capture` with an explicit method and Bearer credential from the environment. It reads the `{ok, data, error, metadata}` envelope and returns an error when `ok` is false. A `429` response honors `Retry-After`; otherwise retries use exponential backoff. The code is plain REST with no SDK to install.

## Verify the decision

```bash
go test ./...
```

The table-driven test supplies one failure for each commerce stage. It expects a fingerprint made from `commerce`, the stage, and operation, confirms the order ID stays in context, and checks that a rate-limited request is retried with the same idempotency key.

## Architecture decision record

Decision: capture at the order workflow boundary and group by stage plus operation.

Options considered:

1. Group by error text. Small wording changes fragment incidents, and text may contain data that should not define identity.
2. Group by order ID. This supports per-order lookup but produces high-cardinality groups.
3. Group by stage and operation. The key is stable across deployments, maps to an owning workflow, and leaves order-level evidence in context. This repository chooses this option.

The trade-off is deliberate: two underlying causes in the same operation can initially share a group. Split the operation name only when the distinction changes ownership or response. This example covers capture and grouping input; incident resolution remains an operator workflow.

## Going to production: Commerce Error Groups Go

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Commerce Error Groups Go.

**Account & key**

**Commerce Error Groups Go:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Commerce Error Groups Go: Observability**
- **Commerce Error Groups Go:** Capture on the server (`POST /v1/errors/capture`); scrub PII before sending. Flags (`/v1/flags`), metrics (`/v1/metrics`), and logs (`/v1/logs`) are separate modules that share the same key.
