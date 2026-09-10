package commerceerrors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestCaptureGroupsByStageAndOperation(t *testing.T) {
	tests := []struct {
		name      string
		failure   Failure
		wantPrint []string
	}{
		{"checkout authorization", Failure{Checkout, "authorize_payment", "ord-101", "issuer declined"}, []string{"commerce", "checkout", "authorize_payment"}},
		{"fulfillment allocation", Failure{Fulfillment, "allocate_stock", "ord-102", "warehouse allocation failed"}, []string{"commerce", "fulfillment", "allocate_stock"}},
		{"receipt delivery", Failure{Receipt, "send_receipt", "ord-103", "receipt delivery failed"}, []string{"commerce", "receipt", "send_receipt"}},
		{"customer update", Failure{OrderUpdate, "notify_customer", "ord-104", "customer notification failed"}, []string{"commerce", "order_update", "notify_customer"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got capturePayload
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Idempotency-Key") != "evt-77" {
					t.Fatalf("request boundary mismatch")
				}
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Fatal(err)
				}
				if calls == 1 {
					w.Header().Set("Retry-After", "1")
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true,"data":{"event_id":"evt-77"},"error":null,"metadata":{"vendor":"infrai"}}`))
			}))
			defer server.Close()

			client := NewClient("test-key")
			client.URL = server.URL
			client.Sleep = func(context.Context, time.Duration) error { return nil }
			if _, err := client.Capture(context.Background(), tt.failure, "evt-77"); err != nil {
				t.Fatal(err)
			}
			if calls != 2 || !reflect.DeepEqual(got.Fingerprint, tt.wantPrint) {
				t.Fatalf("calls=%d fingerprint=%v", calls, got.Fingerprint)
			}
			if got.Context["order_id"] != tt.failure.OrderID {
				t.Fatalf("order context missing: %v", got.Context)
			}
		})
	}
}
