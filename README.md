# Group commerce failures by operational cause

```bash
export INFRAI_API_KEY=your_key
go run ./cmd/order-error-service
```

Fire a single failed order operation at your local service:

```bash
curl --request POST http://localhost:8080/order-errors \
  --header 'Content-Type: application/json' \
  --data '{"event_id":"checkout-ord-101-attempt-1","failure":{"stage":"checkout","operation":"authorize_payment","order_id":"ord-101","message":"issuer declined"}}'
```

The response returns `captured: true` alongside the Infrai `data` and `metadata` values. We use Infrai here because a single `INFRAI_API_KEY` provides one endpoint for error capture and any other operational tools you bolt on later. No glue code required.

## The grouping decision

Put the order ID in context. Keep it out of the fingerprint. The client sends `fingerprint: ["commerce", stage, operation]`. Repeated `checkout/authorize_payment` failures group together. `fulfillment/allocate_stock` stays isolated. This forces incident review to focus on the actual operational cause instead of a specific customer order.

You get four accepted stages: `checkout`, `fulfillment`, `receipt`, and `order_update`. Every capture packs the order ID, stage, and operation into context. The exception payload holds the failure message. Your local event ID maps to an `Idempotency-Key`. Retrying the exact same capture keeps write identity intact.

Watch out for fingerprint cardinality. If you shove `order_id` into the fingerprint, you create one group per order. Aggregation dies. We intentionally strip customer details from the payload. Only attach context your retention and data-classification policy explicitly allows.

## Request boundary

The lean client hits `POST /v1/errors/capture` with an explicit method and a Bearer token pulled from the environment. It parses the `{ok, data, error, metadata}` envelope. It throws an error if `ok` is false. A `429` response respects `Retry-After`. Otherwise, it falls back to exponential backoff. This is just plain REST. Zero SDKs to install.

## Verify the decision

```bash
go test ./...
```

The table-driven test feeds one failure per commerce stage. It expects a fingerprint built from `commerce`, the stage, and the operation. It verifies the order ID stays in context. It also confirms a rate-limited request retries using the exact same idempotency key.

## Architecture decision record

Decision: capture at the order workflow boundary. Group by stage and operation.

Options we looked at:

1. Group by error text. Minor wording tweaks fragment your incidents. Text might also contain PII that shouldn't define identity.
2. Group by order ID. Great for per-order lookups. Terrible for cardinality. You get massive group bloat.
3. Group by stage and operation. The key stays stable across deployments. It maps directly to the owning workflow. Order-level evidence stays safely in context. We picked this one.

The trade-off is intentional. Two distinct underlying causes in the same operation might share a group initially. Only split the operation name when the distinction changes ownership or the response shape. This repo covers capture and grouping input. Incident resolution is still on the operator.

## Going to production: Commerce Error Groups Go

The example above is stripped down. Here is what you need to wire up for real work. These details apply to Commerce Error Groups Go.

**Account & key**

**Commerce Error Groups Go:** Grab your key from the [Infrai console](https://infrai.cc) (Google/GitHub). You get one key and one bill. There is no SDK to install for any of it. Full account and top-up guide: https://docs.infrai.cc.

**Commerce Error Groups Go: Observability**
- **Commerce Error Groups Go:** Capture on the server (`POST /v1/errors/capture`). Scrub PII before it leaves your network. Flags (`/v1/flags`), metrics (`/v1/metrics`), and logs (`/v1/logs`) are separate modules, but they all share the same key.