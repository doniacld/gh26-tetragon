module github.com/doniacld/gh26-tetragon

go 1.26

require github.com/octopkg/upload v0.0.0

// Vendored and pinned (reviewed third-party code lives under third_party/).
replace github.com/octopkg/upload => ./third_party/octopkg-upload
