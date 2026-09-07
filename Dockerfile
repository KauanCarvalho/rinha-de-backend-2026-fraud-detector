# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o /out/indexbuilder ./cmd/indexbuilder


FROM build AS indexer

RUN /out/indexbuilder -input resources/references.json.gz -output /out/index.bin

FROM gcr.io/distroless/static-debian12:nonroot AS final

WORKDIR /

COPY --from=indexer /out/api /api
COPY --from=indexer /out/index.bin /data/index.bin

EXPOSE 9999
USER nonroot:nonroot
ENTRYPOINT ["/api"]
