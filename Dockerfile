# Stage 1: Build the React Frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# Stage 2: Compile the Go Kernel
FROM golang:1.23-alpine AS backend-builder
WORKDIR /app
COPY server/go.mod server/go.sum ./server/
WORKDIR /app/server
RUN go mod download

# Copy the generated frontend dist from Stage 1 into the server folder for go:embed
COPY --from=frontend-builder /app/server/dist ./dist
# Copy the rest of the Go source
COPY server/ .
# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o kernos .

# Stage 3: Minimal Runtime
FROM alpine:latest
RUN apk --no-cache add ca-certificates curl bash git nodejs python3 zip unzip
WORKDIR /root/
COPY --from=backend-builder /app/server/kernos .

EXPOSE 8080

CMD ["./kernos", "-no-browser"]
