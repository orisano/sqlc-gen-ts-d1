# sqlc-gen-ts-d1

https://github.com/voluntas/sqlc-gen-ts-d1-spec

を元に作られたプロトタイプです。

## 使用方法

sqlc v1.19.0 以上で動作します。

sqlc.json の plugins 以下に ts-d1 を追加してください。

v0.0.0-a リリースは main branch に合わせて再生成されているので sha256 を再取得しないと期待通りの動作をしないかもしれません
```bash
cat <<EOS
{
    "name": "ts-d1",
    "wasm": {
        "url": "https://github.com/orisano/sqlc-gen-ts-d1/releases/download/v0.0.0-a/sqlc-gen-ts-d1.wasm",
        "sha256": "$(curl -sSL https://github.com/orisano/sqlc-gen-ts-d1/releases/download/v0.0.0-a/sqlc-gen-ts-d1.wasm.sha256)"
    }
}
EOS
```

### オプション
plugin のオプションにはカンマ区切りの `key=value` 形式文字列を渡すことができます。

* `workers-types-v3=1`: `@cloudflare/workers-types` の v3 のために import 文を出力しないようになります (デフォルトは0)
* `workers-types=2022-11-30`: `@cloudflare/workers-types` の v4 の import する細かいバージョンを指定できます (デフォルトは2022-11-30)

## License
MIT
