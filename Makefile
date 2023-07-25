.PHONY: generate
generate: sqlc.json
	sqlc generate

.PHONY: release
release:
	gh release delete -y --cleanup-tag v0.0.0-a
	gh release create v0.0.0-a bin/sqlc-gen-ts-d1.wasm bin/sqlc-gen-ts-d1.wasm.sha256

sqlc.json: bin/sqlc-gen-ts-d1.wasm.sha256 _sqlc.json
	cat _sqlc.json | WASM_SHA256=$$(cat $<) envsubst > $@

bin/sqlc-gen-ts-d1.wasm.sha256: bin/sqlc-gen-ts-d1.wasm
	openssl sha256 $< | awk '{print $$2}' > $@

bin/sqlc-gen-ts-d1.wasm: cmd/sqlc-gen-ts-d1
	mkdir -p bin && GOROOT=$$(go env GOROOT) tinygo build -o $@ -gc=leaking -scheduler=none -target=wasi -no-debug -ldflags="-X main.version=v0.0.0-a -X main.revision=$$(git rev-parse HEAD)" ./$<

