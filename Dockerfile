# Builds the frontend and the backend, then ships both in one small image:
# the Go binary serves the API and the compiled React app from the same origin.

FROM node:20-alpine AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.23-alpine AS backend
WORKDIR /src
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /out/server /server
COPY --from=frontend /src/dist /public
ENV PORT=8080 STATIC_DIR=/public
EXPOSE 8080
ENTRYPOINT ["/server"]
