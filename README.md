# sqlc-gen-typescript-d1

https://gist.github.com/voluntas/e9516823c5223aac5b61ba51174437fd

を元に作られたプロトタイプで完全には動作しません。

## How to use

https://github.com/orisano/sqlc を使う必要があります

sqlc.json の plugins 以下に typescript-d1 を追加する必要があります
```json
{
    "name": "typescript-d1",
    "wasm": {
        "url": "https://github.com/orisano/sqlc-gen-typescript-d1/releases/download/v0.0.0-a/sqlc-gen-typescript-d1.wasm",
        "sha256": "2f51d44459182959d9eda05409ae54919225866d2a2469fed786cbad6d0e8e65"
    }
}
```

## License
MIT