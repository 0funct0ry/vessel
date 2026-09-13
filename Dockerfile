### build stage ###############################################################
FROM node:20-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 go build -tags embed -ldflags "\
    -s -w \
    -X github.com/0funct0ry/vessel/internal/version.Version=${VERSION} \
    -X github.com/0funct0ry/vessel/internal/version.Commit=${COMMIT} \
    -X github.com/0funct0ry/vessel/internal/version.Date=${DATE}" \
    -o /out/vessel .

### final stage ################################################################
# distroless/static, not scratch: webhook delivery (internal/webhook/sender.go)
# POSTs to user-supplied URLs, which may be https, so the image needs CA certs.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/vessel /vessel
EXPOSE 7373
ENTRYPOINT ["/vessel"]
CMD ["serve", "--addr", "0.0.0.0"]
