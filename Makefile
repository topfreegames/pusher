run:
	@go run main.go
build:
	@mkdir -p bin
	@go build -o bin/pusher main.go
