// Command payments is the ticket-payment service for Mona's After-Party. It
// charges a card for a party ticket and holds the merchant secret ("the vault").
//
// Only the frontend is supposed to talk to it (POST /charge to buy a ticket).
// /vault is an internal endpoint that returns the merchant secret — the thing a
// real attacker pivots to. In the demo the gallery's malicious upload dependency
// reaches /vault directly, which is the east-west flow Tetragon catches.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
)

// vault is the merchant secret. Real deployments mount it from a Secret; here it
// comes from env with a throwaway default so nothing sensitive is baked in.
func vault() string {
	if v := os.Getenv("PAYMENTS_SECRET"); v != "" {
		return v
	}
	return "sk_live_octocat_DEMO_do_not_use_0e8f2c" // obviously fake
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })

	// Buy a ticket. The legitimate, expected caller is the frontend. The charge
	// is simulated: any card is approved and gets a fresh ticket number.
	mux.HandleFunc("/charge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		c := charge{AmountCents: 1000, CardLast4: "4242"} // defaults for a bare POST
		_ = json.NewDecoder(r.Body).Decode(&c)
		c.Status = "paid"
		c.Ticket = fmt.Sprintf("OCTO-%04d", tickets.Add(1))
		log.Printf("charged $%d.%02d to card ending %s, ticket %s", c.AmountCents/100, c.AmountCents%100, c.CardLast4, c.Ticket)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(c)
	})

	// Internal: returns the merchant secret. Nothing in the gallery tier should
	// ever reach this.
	mux.HandleFunc("/vault", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(vault()))
	})

	addr := ":" + getenv("PORT", "8080")
	log.Println("payments listening on", addr)
	log.Fatal(http.ListenAndServe(addr, logRequests(mux)))
}

// charge is both the /charge request (amount + card) and its receipt.
type charge struct {
	AmountCents int    `json:"amount_cents"`
	CardLast4   string `json:"card_last4"`
	Status      string `json:"status,omitempty"`
	Ticket      string `json:"ticket,omitempty"`
}

var tickets atomic.Int64 // ticket numbers handed out so far

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// statusWriter records the response code so the request log can show it.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// logRequests prints one line per request (method, path, status).
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if r.URL.Path != "/healthz" { // probes would drown out the demo's requests
			log.Printf("%s %s -> %d", r.Method, r.URL.Path, sw.status)
		}
	})
}
