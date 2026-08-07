# The web bundle is architecture-independent. Build it on the native builder
# platform so an Apple Silicon builder does not have to emulate amd64 Bun.
FROM --platform=$BUILDPLATFORM oven/bun:1.3.14-alpine@sha256:5acc90a93e91ff07bf72aa90a7c9f0fa189765aec90b47bdbf2152d2196383c0 AS web-build
WORKDIR /src
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
COPY index.html tsconfig.json tsconfig.app.json vite.config.ts biome.json components.json ./
COPY src ./src
COPY public ./public
COPY content ./content
COPY scripts ./scripts
RUN bun run build
RUN mkdir -p /out/fonts \
    && cp node_modules/@fontsource/literata/files/literata-cyrillic-400-normal.woff2 node_modules/@fontsource/literata/files/literata-latin-400-normal.woff2 /out/fonts/ \
    && cp node_modules/@fontsource/source-serif-4/files/source-serif-4-cyrillic-400-normal.woff2 node_modules/@fontsource/source-serif-4/files/source-serif-4-latin-400-normal.woff2 /out/fonts/ \
    && cp node_modules/@fontsource/ibm-plex-mono/files/ibm-plex-mono-cyrillic-400-normal.woff2 node_modules/@fontsource/ibm-plex-mono/files/ibm-plex-mono-latin-400-normal.woff2 /out/fonts/ \
    && cp node_modules/@fontsource/inter/files/inter-cyrillic-400-normal.woff2 node_modules/@fontsource/inter/files/inter-latin-400-normal.woff2 /out/fonts/

# Cross-compile the statically linked Go server instead of emulating the
# target CPU while compiling. BuildKit supplies TARGETOS and TARGETARCH.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS api-build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/writers-studio ./cmd/server

FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce
RUN apk add --no-cache ca-certificates chromium curl fontconfig font-noto freetype harfbuzz nss \
    && addgroup -S app && adduser -S app -G app
ENV CHROMIUM_PATH=/usr/bin/chromium-browser
ENV EXPORT_FONT_DIR=/app/fonts
ENV MERMAID_JS_PATH=/app/mermaid.min.js
WORKDIR /app
COPY --from=api-build /out/writers-studio /app/writers-studio
COPY --from=web-build /src/dist /app/web
COPY --from=web-build /out/fonts /app/fonts
COPY --from=web-build /src/node_modules/mermaid/dist/mermaid.min.js /app/mermaid.min.js
USER app
EXPOSE 8080
ENTRYPOINT ["/app/writers-studio"]
