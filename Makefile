.PHONY: build deploy-development deploy-staging deploy-production test lint configure

build-development:
	STAGE=development make build

build-staging:
	STAGE=staging make build

build-production:
	STAGE=production make build

build: configure
	cd status-checks && env GOARCH=amd64 GOOS=linux CGO_ENABLED=0 go build -ldflags="-s -w" -o ../bootstrap

configure:
	cp config/applications.${STAGE}.toml ./status-checks/applications.toml

deploy-development:
	STAGE=development RATE="rate(1 hour)" serverless deploy --verbose

deploy-staging:
	STAGE=staging RATE="rate(1 hour)" serverless deploy --verbose

deploy-production:
	STAGE=production RATE="rate(20 minutes)" serverless deploy --verbose

test: build-development
	cd status-checks && go test ./...

lint:
	cd status-checks && golangci-lint run

tidy:
	cd status-checks && go mod tidy

lint-install:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sudo sh -s -- -b $(go env GOPATH)/bin v1.54.2
