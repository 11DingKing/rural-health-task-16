FROM golang:1.26

ENV GOTOOLCHAIN=local
WORKDIR /app

# Install Node.js 20 via NodeSource
RUN apt-get update && apt-get install -y curl ca-certificates && \
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt-get install -y nodejs && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Download Go dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Install frontend dependencies and build
COPY web/package*.json ./web/
RUN cd web && npm install

# Build Go and frontend
RUN go build ./...
RUN cd web && npm run type-check && npm run build

CMD ["bash"]
