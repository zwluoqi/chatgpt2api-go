ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG TARGETARCH

FROM --platform=$BUILDPLATFORM node:22-alpine AS web-build

WORKDIR /app/web

COPY web/package.json web/bun.lock ./
RUN npm install

COPY VERSION /app/VERSION
COPY web ./
RUN NEXT_PUBLIC_APP_VERSION="$(cat /app/VERSION)" npm run build

FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS go-build

ARG TARGETPLATFORM
ARG TARGETARCH

WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o chatgpt2api .

FROM --platform=$TARGETPLATFORM alpine:3.20 AS app

WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=go-build /app/chatgpt2api .
COPY --from=web-build /app/web/out ./web_dist
COPY config.json VERSION ./
RUN mkdir -p /app/data
EXPOSE 80
CMD ["./chatgpt2api"]
