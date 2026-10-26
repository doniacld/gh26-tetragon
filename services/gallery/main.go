// Command gallery serves the Octocat photo wall for Mona's After-Party.
package main

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
)

// sighting is one photo on the wall.
type sighting struct {
	Caption string `json:"caption"`
	By      string `json:"by"`
	Image   string `json:"image,omitempty"` // data: URI
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

// Mona's own posts.
var wall = []sighting{
	{Caption: "Mona at the keynote", By: "Mona Lisa Octocat", Image: pic("keynote")},
	{Caption: "Eight arms, eight lanyards", By: "Hubot", Image: pic("lanyards")},
	{Caption: "Spotted: Mona raiding the sticker table", By: "Sealicorn", Image: pic("stickers")},
	{Caption: "Mona vs. the espresso machine", By: "Pufferpup", Image: pic("espresso")},
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })

	// The photo wall (Mona's posts).
	mux.HandleFunc("/pics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wall)
	})

	addr := ":" + getenv("PORT", "8080")
	log.Println("gallery listening on", addr)
	log.Fatal(http.ListenAndServe(addr, logRequests(mux)))
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
		if r.URL.Path != "/healthz" { // skip probe noise
			log.Printf("%s %s -> %d", r.Method, r.URL.Path, sw.status)
		}
	})
}
