.PHONY: build test vet smoke analysis clean
build:
	mkdir -p bin
	go build -trimpath -o bin/vtrace ./cmd/vtrace
test:
	go test ./...
vet:
	go vet ./...
smoke: build
	bash scripts/smoke.sh
analysis:
	python3 -m unittest discover -s analysis/tests
clean:
	rm -f bin/vtrace
