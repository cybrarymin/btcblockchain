-include .envrc

## help: print makefile help
.PHONY: help
help: 
	@echo "Usage:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -n 's/^/  /p'

#===================================================#
# DEVELOPMENT
#===================================================#
current_time = $(shell date +"%Y-%m-%dT%H:%M:%S%z")
git_version = $(shell git describe --always --long --dirty --tags 2>/dev/null; if [[ $$? != 0 ]]; then git describe --always --dirty; fi) # --dirty will add a -dirty to the end of tag or commit shaw that u are already on if there is some uncommited work
Linkerflags = "-s -X github.com/cybrarymin/log-commiter/cmd.buildTime=${current_time} -X github.com/cybrarymin/log-commiter/cmd.version=${git_version}"

## proto: create the proto code and files
.PHONY: proto
proto:
#	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@protoc -I ./ --go_out=./ --go-grpc_out=./ proto/types/*.proto proto/*.proto


## build: build the linux and mac binary of the application
.PHONY: build
build:
	@go mod tidy
	@protoc -I ./ --go_out=./ --go-grpc_out=./ proto/types/*.proto proto/*.proto
	@GOARCH="amd64" GOOS="linux" go build -ldflags=${Linkerflags} -o ./bin/log-commiter-amd64-linux
	@GOARCH="arm64" GOOS="darwin" go build -ldflags=${Linkerflags} -o ./bin/log-commiter-arm64-mac


## run/bootstrap: run the application
.PHONY: run/bootstrap
run/bootstrap:
	@go run main.go server \
	--jaeger-service-name="boostrap-01" \
	--log-level=debug \
	--bootstrap=true \
	--grpc-listen-address="127.0.0.1" \
	--grpc-listen-port="6881" \
	--chain="customChain"
	

## run/peer: run the application
.PHONY: run/peer
run/peer:
	@go run main.go server \
	--jaeger-service-name="peer-01" \
	--log-level=debug \
	--bootstrap=false \
	--grpc-listen-address="127.0.0.1" \
	--grpc-listen-port=`jot -r 1 6882 6889` \
	--bootstrap-addr="127.0.0.1:6881" \
	--keystore-dir="/tmp/keystore" \
	--blockstore-dir="/tmp/blockstore"


#===================================================#
# QUALITY CHECK, LINTING, SECURITY CHECK, Vendoring
#===================================================#
## audit: verify and download the packages
.PHONY: audit
audit:
	@echo "Verifying and downloading the packages..."
	@go mod tidy
	@go mod verify
	@echo "Formatting code..."
	@go fmt ./...
	@echo "code quality check...."
	@go vet ./...
	@staticcheck ./...
	@echo "running unit tests"
	@go test -race -vet=off ./...

## vendor: vendor and store all the packages locally
.PHONY: vendor
vendor:
	@echo "Tidying and verifying golang packages and module dependencies..."
	go mod verify
	go mod tidy
	@echo "Vendoring all golang dependency modules and packages..."
	go mod vendor
