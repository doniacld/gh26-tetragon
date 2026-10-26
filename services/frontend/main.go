// Command frontend is the public site for Mona's After-Party: browse the Octocat
// photo wall, upload your own sighting, and buy a party ticket.
//
// It is the only tier that fans out east-west: it calls the gallery service
// (/pics, /upload) and the payments service (/charge).
package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed templates
var templatesFS embed.FS

var pages = map[string]*template.Template{
	"home":    page("home.html"),
	"gallery": page("gallery.html"),
	"tickets": page("tickets.html"),
}

func page(name string) *template.Template {
	return template.Must(template.ParseFS(templatesFS, "templates/layout.html", "templates/"+name))
}

type pageData struct {
	Page      string
	Sightings []sighting
	Status    string
	Error     bool // Status is a failure (rendered red)
	Price     string
	Receipt   *receipt
}

// ticketPriceCents is what one after-party ticket costs.
const ticketPriceCents = 1000

// receipt is what payments returns for a successful (simulated) charge.
type receipt struct {
	AmountCents int    `json:"amount_cents"`
	CardLast4   string `json:"card_last4"`
	Ticket      string `json:"ticket"`
}

func (r receipt) Amount() string { return dollars(r.AmountCents) }

func dollars(cents int) string { return fmt.Sprintf("$%d.%02d", cents/100, cents%100) }

type sighting struct {
	Caption string `json:"caption"`
	By      string `json:"by"`
	Image   string `json:"image,omitempty"`
}

// ImageURL marks an uploaded thumbnail's data: URI as safe for <img src>. Only
// data:image/* is trusted; anything else renders no image.
func (s sighting) ImageURL() template.URL {
	if strings.HasPrefix(s.Image, "data:image/") {
		return template.URL(s.Image)
	}
	return ""
}

func galleryAddr() string  { return getenv("GALLERY_ADDR", "gallery:8080") }
func paymentsAddr() string { return getenv("PAYMENTS_ADDR", "payments:8080") }

var httpClient = &http.Client{Timeout: 3 * time.Second}

func render(w http.ResponseWriter, name string, data pageData) {
	renderCode(w, name, data, http.StatusOK)
}

// renderCode renders a page with an explicit status code. Content-Type is set
// before WriteHeader so the status (e.g. 503 when the gallery upload is blocked)
// is honoured and visible to curl, while the themed page still shows the reason.
func renderCode(w http.ResponseWriter, name string, data pageData, code int) {
	data.Page = name
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	_ = pages[name].ExecuteTemplate(w, "layout", data)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/", home)
	mux.HandleFunc("/gallery", gallery)
	mux.HandleFunc("/upload", upload) // proxies to the gallery service
	mux.HandleFunc("/tickets", tickets)
	mux.HandleFunc("/buy", buy) // proxies to the payments service

	addr := ":" + getenv("PORT", "8080")
	log.Println("frontend listening on", addr)
	log.Fatal(http.ListenAndServe(addr, logRequests(mux)))
}

func home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	render(w, "home", pageData{})
}

// gallery renders Mona's photo wall.
func gallery(w http.ResponseWriter, _ *http.Request) {
	render(w, "gallery", pageData{Sightings: fetchSightings()})
}

// fetchSightings gets the photo wall (Mona's posts + uploads) from the gallery service.
func fetchSightings() []sighting {
	var pics []sighting
	resp, err := httpClient.Get("http://" + galleryAddr() + "/pics")
	if err == nil {
		defer resp.Body.Close()
		_ = json.NewDecoder(resp.Body).Decode(&pics)
	}
	return pics
}

// upload forwards the chosen sighting to the gallery service's /upload. This
// It re-renders the photo wall so the new sighting shows up right away.
func upload(w http.ResponseWriter, r *http.Request) {
	resp, err := httpClient.Post("http://"+galleryAddr()+"/upload", r.Header.Get("Content-Type"), r.Body)
	status, code := "Thumbnail ready! Mona added it to the wall.", http.StatusOK
	failed := err != nil || resp.StatusCode >= 400
	if failed {
		status = "Upload blocked: the thumbnail service refused (HTTP " + statusOf(resp, err) + ")."
		code = http.StatusServiceUnavailable
	}
	if resp != nil {
		resp.Body.Close()
	}
	renderCode(w, "gallery", pageData{Sightings: fetchSightings(), Status: status, Error: failed}, code)
}

func tickets(w http.ResponseWriter, _ *http.Request) {
	render(w, "tickets", pageData{Price: dollars(ticketPriceCents)})
}

// buy forwards a ticket purchase to the payments service. This is the legitimate
// frontend->payments flow. Only the card's last 4 digits leave the frontend.
func buy(w http.ResponseWriter, r *http.Request) {
	body, _ := json.Marshal(receipt{AmountCents: ticketPriceCents, CardLast4: last4(r.FormValue("card"))})
	data := pageData{Price: dollars(ticketPriceCents), Status: "Payment failed. Please try again."}
	resp, err := httpClient.Post("http://"+paymentsAddr()+"/charge", "application/json", bytes.NewReader(body))
	if err == nil {
		defer resp.Body.Close()
		var rc receipt
		if resp.StatusCode == http.StatusCreated && json.NewDecoder(resp.Body).Decode(&rc) == nil {
			data.Receipt, data.Status = &rc, ""
		}
	}
	render(w, "tickets", data)
}

// last4 returns the last four digits of a card number ("4242" if none given).
func last4(card string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, card)
	if len(digits) < 4 {
		return "4242"
	}
	return digits[len(digits)-4:]
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func statusOf(resp *http.Response, err error) string {
	if err != nil || resp == nil {
		return "000"
	}
	return strconv.Itoa(resp.StatusCode)
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

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if r.URL.Path != "/healthz" { // skip probe noise
			log.Printf("%s %s -> %d", r.Method, r.URL.Path, sw.status)
		}
	})
}
