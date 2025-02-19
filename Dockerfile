# Start from the latest golang base image
FROM golang:1.23.0 as builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app for multiple platforms
# Example: Linux
RUN GOOS=linux GOARCH=amd64 go build -o ./bin/disk-usage-linux-amd64 ./main.go
# macOS Intel
RUN GOOS=darwin GOARCH=amd64 go build -o ./bin/disk-usage-darwin-amd64 ./main.go
# macOS Apple M1
RUN GOOS=darwin GOARCH=arm64 go build -o ./bin/disk-usage-darwin-arm64 ./main.go

