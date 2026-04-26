FROM golang:1.23-bookworm AS builder

WORKDIR /src

# Cache dependencies in a stable layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/tournament-sync ./

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /
COPY --from=builder /out/tournament-sync /tournament-sync

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/tournament-sync"]
