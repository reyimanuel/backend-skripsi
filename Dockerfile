# syntax=docker/dockerfile:1

FROM golang:1.25.6-bookworm AS builder

WORKDIR /src

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Build only from source directories needed by the Go binary.
# Keeping docs, generated files, and local artifacts out of this layer makes remote builds leaner.
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/server


FROM debian:bookworm-slim AS runtime

# LibreOffice for DOCX->PDF conversion, Tesseract OCR for KRS extraction, + basic fonts
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates tzdata \
        libreoffice-writer libreoffice-core libreoffice-common \
        tesseract-ocr tesseract-ocr-ind \
        fontconfig fonts-dejavu-core fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# App binary
COPY --from=builder /out/app /app/app

# Static assets (templates, example images)
COPY public/ /app/public/

# Runtime dirs (uploads, temp)
RUN mkdir -p \
    /app/public/generated/attachments \
    /app/public/images/profile-photos \
    /app/public/images/signatures \
    /app/public/images/student-cards \
    /app/tmp \
    /app/keys

# Entrypoint that can materialize secrets from env
COPY deploy/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Run as non-root
RUN useradd -r -u 10001 -g nogroup appuser \
    && chown -R appuser:nogroup /app /entrypoint.sh
USER appuser

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/app/app"]
