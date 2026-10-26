// Command gallery serves the Octocat photo wall for Mona's After-Party and the
// feature a developer just shipped with help from an AI agent: "upload your own
// Octocat sighting". The upload path calls github.com/octopkg/upload — the handy
// image helper the agent picked — to make a thumbnail.
//
// That dependency is the wrong one. On every upload it also opens a connection
// from this workload to the payments service and scrapes its vault. The gallery
// tier has no business talking to payments, which is exactly what makes the flow
// easy to spot at runtime.
package main

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg" // register formats for image.DecodeConfig
	_ "image/png"
	"log"
	"net/http"
	"os"

	"github.com/octopkg/upload"
)

// sighting is one photo on the wall.
type sighting struct {
	Caption string `json:"caption"`
	By      string `json:"by"`
	Image   string `json:"image,omitempty"` // data: URI, set for uploaded sightings
	ID      string `json:"id,omitempty"`    // set for uploads (moderation)
}

//go:embed pics
var picsFS embed.FS

// pic returns an embedded illustration from pics/ as a data: URI.
func pic(name string) string {
	b, err := picsFS.ReadFile("pics/" + name + ".svg")
	if err != nil {
		log.Fatal(err)
	}
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(b)
}

// Mona's own posts — the benign content that's there before anyone uploads.
var wall = []sighting{
	{Caption: "Mona at the keynote", By: "Mona Lisa Octocat", Image: pic("keynote")},
	{Caption: "Eight arms, eight lanyards", By: "Hubot", Image: pic("lanyards")},
	{Caption: "Spotted: Mona raiding the sticker table", By: "Sealicorn", Image: pic("stickers")},
	{Caption: "Mona vs. the espresso machine", By: "Pufferpup", Image: pic("espresso")},
}

// uploads holds guests' sightings: pending until approved (see moderation.go).
var uploads queue

const (
	maxUploadBytes  = 10 << 20 // 10 MB per photo
	maxUploadPixels = 40e6     // refuse decompression bombs before decoding
)

func main() {
	uploads.file = os.Getenv("UPLOADS_FILE")
	if err := uploads.load(); err != nil {
		log.Printf("loading uploads from %s: %v (starting empty)", uploads.file, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })

	// The photo wall: approved uploads, then Mona's posts.
	mux.HandleFunc("/pics", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, append(uploads.wall(), wall...))
	})

	// Moderation, called by the frontend's /admin page (in-cluster only).
	mux.HandleFunc("/pending", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, uploads.waiting())
	})
	mux.HandleFunc("/approve", moderate(uploads.approve))
	mux.HandleFunc("/reject", moderate(uploads.reject))
	mux.HandleFunc("/remove", moderate(uploads.remove)) // take an approved photo off the wall
	mux.HandleFunc("/approve-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]int{"approved": uploads.approveAll()})
	})

	// The new feature: upload your own sighting. We make a thumbnail via the
	// agent-chosen helper — which is where the payload fires on every call.
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
		f, _, err := r.FormFile("sighting")
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				http.Error(w, "photo too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(f); err != nil {
			http.Error(w, "photo too large", http.StatusRequestEntityTooLarge)
			return
		}
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(buf.Bytes())); err != nil || cfg.Width*cfg.Height > maxUploadPixels {
			http.Error(w, "not a JPEG or PNG photo", http.StatusBadRequest)
			return
		}

		thumb, err := upload.Thumbnail(buf.Bytes())
		if err != nil {
			// If the runtime blocks the payload's syscall, the helper returns an
			// error and this endpoint fails closed. Browsing /pics stays fine.
			http.Error(w, "thumbnail unavailable", http.StatusServiceUnavailable)
			return
		}
		uploads.add("data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(thumb))
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(thumb)
	})

	addr := ":" + getenv("PORT", "8080")
	log.Println("gallery listening on", addr)
	log.Fatal(http.ListenAndServe(addr, logRequests(mux)))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// moderate wraps approve/reject: POST with form value id.
func moderate(act func(id string) bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if !act(r.FormValue("id")) {
			http.Error(w, "no such pending sighting", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

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
