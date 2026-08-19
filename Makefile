.PHONY: build test test-race vet fmt clean frontend

build:
	go build ./...

test:
	go test -timeout=300s -count=1 ./...

test-race:
	go test -race -timeout=420s -count=1 ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

clean:
	rm -rf data/ web/dist/

frontend:
	cd web && npm install && npm run type-check && npm run build

run-gateway:
	go run ./cmd/certgateway -config configs/config.yaml

run-upstream:
	go run ./cmd/certupstream -addr :52662 -name "Primary Testing Center"

run-admin:
	go run ./cmd/certadmin submissions

docker-build:
	docker build --platform linux/amd64 -f eval.Dockerfile -t rehabcert-eval .

docker-run:
	docker run -p 52661:52661 rehabcert-eval
