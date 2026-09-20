package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"flashcard_lambda/internal/models"

	"github.com/aws/aws-lambda-go/events"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
)

func TestLargeEscapedResponseFitsLambdaEnvelopeOrReturns413(t *testing.T) {
	for _, tc := range []struct {
		name, question string
		wantStatus     int
	}{
		{"ordinary text", strings.Repeat("x", 32768), http.StatusOK},
		{"escaped text", strings.Repeat("\\", 32768), http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cards := make([]models.Card, 62)
			for i := range cards {
				cards[i].Question = tc.question
			}
			adapter := httpadapter.New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusOK, cards)
			}))
			response, err := adapter.Proxy(events.APIGatewayProxyRequest{HTTPMethod: "GET", Path: "/cards"})
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tc.wantStatus {
				t.Errorf("status=%d; want %d", response.StatusCode, tc.wantStatus)
			}
			envelope, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			if len(envelope) >= 6<<20 {
				t.Fatalf("proxy envelope is %d bytes, above Lambda's 6 MiB limit", len(envelope))
			}
		})
	}
}
