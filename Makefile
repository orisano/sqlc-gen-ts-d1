.PHONY: generate
generate: sqlc.json
	sqlc generate

.PHONY: release
release:
	gh release delete -y v0.0.0-a
	gh release create v0.0.0-a plugin/bin/sqlc-gen-typescript-d1.wasm plugin/bin/sqlc-gen-typescript-d1.wasm.sha256

sqlc.json: plugin/bin/sqlc-gen-typescript-d1.wasm.sha256 _sqlc.json
	cat _sqlc.json | WASM_SHA256=$$(cat $<) envsubst > $@

plugin/bin/sqlc-gen-typescript-d1.wasm.sha256: plugin/bin/sqlc-gen-typescript-d1.wasm
	openssl sha256 $< | awk '{print $$2}' > $@

plugin/bin/sqlc-gen-typescript-d1.wasm: plugin/main.go
	cd plugin && mkdir -p bin && GOROOT=$$(go env GOROOT) tinygo build -o ./bin/sqlc-gen-typescript-d1.wasm -gc=leaking -scheduler=none -target=wasi main.go

