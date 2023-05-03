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
        "sha256": "d70852c9a58d6c30903cc81a7e811e530a69e096c44769223ae98b261184f146"
    }
}
```

## License
MIT