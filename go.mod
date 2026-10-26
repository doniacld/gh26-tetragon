module github.com/doniacld/gh26-tetragon

go 1.26

require github.com/octopkg/upload v0.0.0

// The "dependency" is a LOCAL module (evilupload/). The replace directive means
// nothing is downloaded — no CVE, no live internet calls. The malicious
// behaviour is harmless and stays in-cluster: it's a demo of *unexpected runtime
// behaviour* (gallery -> payments), not an attack.
replace github.com/octopkg/upload => ./evilupload
