# Copyright Corsha Inc. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

all: test simi build-simi-image

lint: lint-go lint-shell lint-helm

lint-go:
	golangci-lint run --timeout 2m

lint-shell:
	shellcheck scripts/*.sh

lint-helm:
	helm lint k8s/helm/simi --values k8s/helm/simi/ci/ci-values.yaml -f k8s/helm/simi/values.yaml

test:
	GO111MODULE=on go test -race -v -coverprofile=coverage.out ./...

tidy:
	GO111MODULE=on go mod tidy

simi:
	GOOS=linux GOARCH=amd64 go build -o bin/simi cmd/simi.go
	GOOS=linux GOARCH=amd64 go build -o benchmark/bench-consumer benchmark/cmd/consumer.go

simi-image:
	docker build -t simi-test:latest bin

consumer-image:
	docker build -t consumer-test:latest benchmark

.PHONY: tidy lint test simi build-simi-image
