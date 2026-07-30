# Stage 1: Build the Go binary
FROM golang:1.23-alpine AS builder

# Install build dependencies, including git and ssh client for fetching private packages
RUN apk add --no-cache git openssh-client

WORKDIR /app

# Create the .ssh directory and add github.com's public key to known_hosts
# This prevents the interactive "Are you sure you want to connect?" prompt
RUN mkdir -p -m 0700 ~/.ssh && ssh-keyscan github.com >> ~/.ssh/known_hosts

# Configure git to use SSH for GitHub instead of HTTPS
RUN git config --global url."git@github.com:".insteadOf "https://github.com/"

# Set GOPRIVATE to specify private module paths
ENV GOPRIVATE="github.com/corshatech/*"

# Copy the dependency files first for efficient caching
COPY go.mod go.sum ./

# Download dependencies using SSH mount for authenticating with private repos
RUN --mount=type=ssh go mod download

# Copy the rest of the source code
COPY . .

# Build the static executable with CGO disabled
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o chaincode server.go

# Stage 2: Final minimal runner image
FROM alpine:3.20

# Create a non-root user with UID 1000 and group with GID 1000 for security best practices
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -D appuser

WORKDIR /app

# Copy the compiled binary from the builder stage and set ownership
COPY --from=builder --chown=appuser:appgroup /app/chaincode .

# Use the non-root user
USER 1000

# Set the ENTRYPOINT to execute the chaincode binary
ENTRYPOINT ["./chaincode"]
