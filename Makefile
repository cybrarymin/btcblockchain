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


## run: run the application
.PHONY: run 
run:
	@go run main.go


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