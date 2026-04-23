BINARY := bump-brat
MAIN := bump-brat.go

.PHONY: build run clean

build:
	go build -o $(BINARY) $(MAIN)

run:
	go run $(MAIN)

clean:
	rm -f $(BINARY)
