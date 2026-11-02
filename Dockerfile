# Two images from one file: `--target api` (small, no demo parser) and
# `--target worker`.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir /uploads

FROM build AS build-api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/csdc-api ./cmd/api

FROM build AS build-worker
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/csdc-worker ./cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot AS worker
COPY --from=build-worker /out/csdc-worker /csdc-worker
ENTRYPOINT ["/csdc-worker"]

FROM gcr.io/distroless/static-debian12:nonroot AS api
COPY --from=build-api /out/csdc-api /csdc-api
# Owned by the nonroot user so the demos volume is writable.
COPY --from=build --chown=65532:65532 /uploads /demos
EXPOSE 8080
ENTRYPOINT ["/csdc-api"]
