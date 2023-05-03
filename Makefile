plugin/bin/sqlc-gen-typescript-d1.wasm.sha256: plugin/bin/sqlc-gen-typescript-d1.wasm
	openssl sha256 $< | awk '{print $$2}' > $@

plugin/bin/sqlc-gen-typescript-d1.wasm: plugin/main.go
	cd plugin && mkdir -p bin && GOROOT=$$(go env GOROOT) tinygo build -o ./bin/sqlc-gen-typescript-d1.wasm -gc=leaking -scheduler=none -target=wasi main.go

