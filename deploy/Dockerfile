# syntax=docker/dockerfile:1
# One image recipe for all three After-Party services. Build with
#   docker build -f deploy/Dockerfile --build-arg SERVICE=gallery \
#     --build-arg GIT_SHA=$(git rev-parse HEAD) -t gallery:demo .
# The binary lands at /usr/local/bin/<service> so each workload has a distinct
# path — that path is what the TracingPolicy's matchBinaries selects on, and
# what shows up as "binary" in every Tetragon event.
FROM golang:1.26 AS build
WORKDIR /src
COPY . .
ARG SERVICE
RUN CGO_ENABLED=0 go build -o /out/${SERVICE} ./services/${SERVICE}

FROM debian:stable-slim
ARG SERVICE
COPY --from=build /out/${SERVICE} /usr/local/bin/${SERVICE}
# Keep the service name available at runtime so the entrypoint can exec the
# right binary by its real path (exec => PID 1 *is* /usr/local/bin/<service>).
ENV SVC=${SERVICE}

# ── SHA attribution ─────────────────────────────────────────────────────────
# CI/Makefile pass --build-arg GIT_SHA=<sha>; baked as the OCI revision label and
# stamped onto the pod (deploy/after-party.yaml) so a Tetragon event maps back to
# a commit.
ARG GIT_SHA=dev
LABEL org.opencontainers.image.revision=$GIT_SHA

EXPOSE 8080
ENTRYPOINT ["/bin/sh", "-c", "exec /usr/local/bin/$SVC"]
