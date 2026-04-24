BINARY := bump-brat
MAIN := bump-brat.go

.PHONY: build run clean test test-verbose

build:
	go build -o $(BINARY) $(MAIN)

run:
	go run $(MAIN)

clean:
	rm -f $(BINARY)

test:
	go test -v

test-verbose:
	go test -v -count=1
