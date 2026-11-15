FROM golang:1.27.4-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY .go-version ./
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY scripts/build.sh ./scripts/build.sh
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN ./scripts/build.sh /out/hospitality

FROM alpine:3.24
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/hospitality /usr/local/bin/hospitality
USER 65532:65532
EXPOSE 8000
ENV HTTP_ADDR=0.0.0.0:8000
ENTRYPOINT ["/usr/local/bin/hospitality"]
