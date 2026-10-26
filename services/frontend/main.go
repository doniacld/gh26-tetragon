// Command frontend is the public site for Mona's After-Party: browse the Octocat
// photo wall, upload your own sighting, and buy a party ticket.
//
// It is the only tier that is supposed to fan out east-west: it calls the
// gallery service (/pics, /upload) and the payments service (/charge). Those are
// the expected flows. The gallery->payments call the upload dependency makes is
// NOT, and that is the whole point of the demo.
package main

import (
	"bytes"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
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
	"admin":   page("admin.html"),
}

func page(name string) *template.Template {
	return template.Must(template.ParseFS(templatesFS, "templates/layout.html", "templates/"+name))
}

type pageData struct {
	Page      string
	Sightings []sighting
	Token     string     // admin page only
	OnWall    []sighting // admin page only: approved uploads
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
	ID      string `json:"id,omitempty"`
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

// uploadClient allows for thumbnailing a full-size phone photo on a small VM.
var uploadClient = &http.Client{Timeout: 20 * time.Second}

const maxUploadBytes = 10 << 20 // 10 MB, same limit as the gallery

// uploadLimit caps uploads for the whole site, so a stranger can't flood the
// demo while it's public. A crowd uploading at once stays well under it.
var uploadLimit = newRateLimit(120, time.Minute)

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
	mux.HandleFunc("/buy", buy)          // proxies to the payments service
	mux.HandleFunc("/api/pics", apiPics) // the wall as JSON, for the live refresh
	mux.HandleFunc("/admin", admin)      // the presenter's moderation page

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
// frontend->gallery hop is expected; the gallery->payments hop it triggers is not.
// The photo then waits for the presenter's approval before it reaches the wall.
func upload(w http.ResponseWriter, r *http.Request) {
	show := func(status string, failed bool, code int) {
		renderCode(w, "gallery", pageData{Sightings: fetchSightings(), Status: status, Error: failed}, code)
	}
	if !uploadLimit.allow() {
		show("Lots of uploads right now! Try again in a minute.", true, http.StatusTooManyRequests)
		return
	}
	// Receive the whole photo first: a phone on venue Wi-Fi may need seconds.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxUploadBytes+1<<20))
	if err != nil {
		show("That photo is too big (10 MB max).", true, http.StatusRequestEntityTooLarge)
		return
	}
	resp, err := uploadClient.Post("http://"+galleryAddr()+"/upload", r.Header.Get("Content-Type"), bytes.NewReader(body))
	if resp != nil {
		resp.Body.Close()
	}
	switch {
	case err == nil && resp.StatusCode == http.StatusCreated:
		show("Thanks! Your sighting is waiting for Mona's approval. 📸", false, http.StatusOK)
	case err == nil && resp.StatusCode == http.StatusBadRequest:
		show("That file isn't a JPEG or PNG photo.", true, http.StatusBadRequest)
	case err == nil && resp.StatusCode == http.StatusRequestEntityTooLarge:
		show("That photo is too big (10 MB max).", true, http.StatusRequestEntityTooLarge)
	default:
		show("Upload blocked: the thumbnail service refused (HTTP "+statusOf(resp, err)+").", true, http.StatusServiceUnavailable)
	}
}

// apiPics serves the wall as JSON for the gallery page's live refresh. Image
// URIs are filtered the same way as in the template.
func apiPics(w http.ResponseWriter, _ *http.Request) {
	type pic struct {
		Caption string `json:"caption"`
		By      string `json:"by"`
		Image   string `json:"image,omitempty"`
	}
	pics := []pic{}
	for _, s := range fetchSightings() {
		pics = append(pics, pic{s.Caption, s.By, string(s.ImageURL())})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(pics)
}

// admin is the presenter's moderation page: pending uploads with approve and
// reject buttons. It is off unless ADMIN_TOKEN is set, and every request must
// carry that token.
func admin(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("ADMIN_TOKEN")
	given := r.FormValue("token")
	if token == "" || subtle.ConstantTimeCompare([]byte(given), []byte(token)) != 1 {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		action := map[string]string{"approve": "/approve", "reject": "/reject",
			"approve-all": "/approve-all", "remove": "/remove"}[r.FormValue("action")]
		if action != "" {
			resp, err := httpClient.PostForm("http://"+galleryAddr()+action, url.Values{"id": {r.FormValue("id")}})
			if err == nil {
				resp.Body.Close()
			}
		}
		http.Redirect(w, r, "/admin?token="+url.QueryEscape(token), http.StatusSeeOther)
		return
	}
	var pending []sighting
	if resp, err := httpClient.Get("http://" + galleryAddr() + "/pending"); err == nil {
		defer resp.Body.Close()
		_ = json.NewDecoder(resp.Body).Decode(&pending)
	}
	var onWall []sighting
	for _, s := range fetchSightings() {
		if s.ID != "" { // uploads have an id; Mona's own posts don't
			onWall = append(onWall, s)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	render(w, "admin", pageData{Sightings: pending, OnWall: onWall, Token: token})
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
		if r.URL.Path != "/healthz" { // probes would drown out the demo's requests
			log.Printf("%s %s -> %d", r.Method, r.URL.Path, sw.status)
		}
	})
}
