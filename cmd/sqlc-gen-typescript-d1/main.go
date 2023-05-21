package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/orisano/sqlc-gen-typescript-d1/codegen/plugin"
)

// handler は sqlc で解析したスキーマとクエリの情報を元に生成するコードの情報を返す
func handler(request *plugin.CodeGenRequest) (*plugin.CodeGenResponse, error) {
	options := parseOption(request.GetPluginOptions())
	workersTypesVersion := "2022-11-30"
	if v, ok := options["workers-types"]; ok {
		workersTypesVersion = v
	}
	workersTypesV3 := false
	if v, ok := options["workers-types-v3"]; ok {
		workersTypesV3 = v == "1"
	}

	tsTypeMap := buildTsTypeMap(request.GetSettings())
	var files []*plugin.File
	{
		// sqlc_embed の際にスキーマの型が必要になるので models.ts として書き出す
		models := bytes.NewBuffer(nil)
		for _, s := range request.GetCatalog().GetSchemas() {
			for _, t := range s.GetTables() {
				modelName := naming.toModelTypeName(t.GetRel())
				fmt.Fprintf(models, "export type %s = {\n", modelName)
				for _, c := range t.GetColumns() {
					colName := naming.toPropertyName(c)
					tsType := tsTypeMap.toTsType(c)
					fmt.Fprintf(models, "  %s: %s;\n", colName, tsType)
				}
				fmt.Fprintf(models, "};\n\n")
			}
		}
		files = append(files, &plugin.File{Name: "models.ts", Contents: models.Bytes()})
	}

	{
		querier := bytes.NewBuffer(nil)

		tableMap := buildTableMap(request.GetCatalog())

		workersTypesPackage := "@cloudflare/workers-types"
		if workersTypesVersion != "" {
			workersTypesPackage += "/" + workersTypesVersion
		}

		header := bytes.NewBuffer(nil)
		if !workersTypesV3 {
			header.WriteString("import { D1Database, D1Result } from \"" + workersTypesPackage + "\"\n")
		}

		requireModels := map[string]bool{}
		requireExpandedParams := false

		for _, q := range request.GetQueries() {
			queryText := q.GetText()
			// sqlc_embed はカラムを x.a, x.b, x.c のような形で展開する
			// 複数の sqlc_embed が展開された結果、重複した名前のカラムの情報が得られない処理系がある
			// そのため x.a AS x_a, x.b AS x_b, x.c AS x_c のようにクエリを書き換えることで問題を回避する
			// カラムを一つずつ書き換えた場合は前方一致や後方一致を考慮する必要があるのでまとめて書き換えを行う
			for _, c := range q.GetColumns() {
				et := c.GetEmbedTable()
				if et.GetName() == "" {
					continue
				}
				var news, olds []string
				for _, ec := range tableMap.findTable(et).GetColumns() {
					from := et.GetName() + "." + ec.GetName()
					to := from + " AS " + naming.toEmbedColumnName(et, ec)
					olds = append(olds, from)
					news = append(news, to)
				}
				queryText = strings.Replace(queryText, strings.Join(olds, ", "), strings.Join(news, ", "), 1)
			}

			query := "-- name: " + q.GetName() + " " + q.GetCmd() + "\n" + queryText
			fmt.Fprintf(querier, "const %s = `%s`;\n", naming.toConstQueryName(q), query)

			querier.WriteByte('\n')

			// パラメータが0個の場合は引数から削除するので型を生成しない
			if len(q.GetParams()) > 0 {
				fmt.Fprintf(querier, "export type %s = {\n", naming.toParamsTypeName(q))
				for _, p := range q.GetParams() {
					c := p.GetColumn()
					paramName := naming.toPropertyName(c)
					tsType := tsTypeMap.toTsType(c)
					// パラメータは sqlc_narg を使った場合のみ nullable
					if c.GetNotNull() {
						// パラメータに対応するカラムがわかっていて、スキーマ上で nullable であればパラメータを nullable とする
						if tc := tableMap.findColumn(c); tc != nil && !tc.GetNotNull() {
							tsType += " | null"
						}
					}
					fmt.Fprintf(querier, "  %s: %s;\n", paramName, tsType)
				}
				querier.WriteString("};\n")

				querier.WriteByte('\n')
			}

			needRawType := false
			// :exec はレスポンスが返ってこないので型を生成しない
			if q.GetCmd() != ":exec" {
				fmt.Fprintf(querier, "export type %s = {\n", naming.toQueryRowTypeName(q))
				for _, c := range q.GetColumns() {
					colName := c.GetName()
					propName := naming.toPropertyName(c)

					// カラム名(snake)とプロパティ名(camel)が異なる場合
					// 生成コードの内部で変換する必要があるのでクエリの内部結果型が必要になる
					if colName != propName {
						needRawType = true
					}

					tsType := ""

					// sqlc_embed が使われている場合
					// 生成コードの内部で変換する必要があるのでクエリの内部結果型が必要になる
					if et := c.GetEmbedTable(); et.GetName() != "" {
						needRawType = true
						tsType = naming.toModelTypeName(et)
						// models.ts から import が必要になる
						requireModels[tsType] = true
					} else {
						tsType = tsTypeMap.toTsType(c)
					}
					fmt.Fprintf(querier, "  %s: %s;\n", propName, tsType)
				}
				querier.WriteString("};\n")

				querier.WriteByte('\n')
			}

			// 內部結果型が必要な場合のみ生成する
			if needRawType {
				fmt.Fprintf(querier, "type %s = {\n", naming.toRawQueryRowTypeName(q))
				for _, c := range q.GetColumns() {
					// sqlc_embed の場合、スキーマからカラムの情報を取得し展開する
					if et := c.GetEmbedTable(); et.GetName() != "" {
						for _, ec := range tableMap.findTable(et).GetColumns() {
							colName := naming.toEmbedColumnName(et, ec)
							tsType := tsTypeMap.toTsType(ec)
							fmt.Fprintf(querier, "  %s: %s;\n", colName, tsType)
						}
					} else {
						colName := c.GetName()
						tsType := tsTypeMap.toTsType(c)
						fmt.Fprintf(querier, "  %s: %s;\n", colName, tsType)
					}
				}
				querier.WriteString("};\n")

				querier.WriteByte('\n')
			}

			rowType := naming.toQueryRowTypeName(q)
			// retType は関数の戻り値の型
			var retType string
			// resultType は SQLite からの戻り値の型
			var resultType string

			if cmd := q.GetCmd(); cmd == ":one" {
				retType = rowType + " | null"
				resultType = retType
				if needRawType {
					resultType = naming.toRawQueryRowTypeName(q) + " | null"
				}
			} else if cmd == ":exec" {
				retType = "D1Result"
			} else {
				retType = "D1Result<" + rowType + ">"
				resultType = rowType
				if needRawType {
					resultType = naming.toRawQueryRowTypeName(q)
				}
			}

			fmt.Fprintf(querier, "export async function %s(\n", naming.toFunctionName(q))
			fmt.Fprintf(querier, "  d1: D1Database")
			// パラメータがないときは引数を追加しない
			if len(q.GetParams()) > 0 {
				querier.WriteString(",\n")
				fmt.Fprintf(querier, "  args: %s", naming.toParamsTypeName(q))
			}
			querier.WriteString("\n")
			fmt.Fprintf(querier, "): Promise<%s> {\n", retType)

			var queryVar string
			var bindArgs string
			if hasSqlcSlice(q) {
				// SQLite はパラメータに配列を指定できないため、sqlc.slice では実行時にクエリを書き換える必要がある
				// sqlc はパラメータに自動採番する都合で sqlc.slice のパラメータは登場順で番号がつく
				// しかし ? には番号がついてない文字列が出力される (kyleconroy/sqlc/pull/2274)
				// 動的にパラメータの数が変動するが他のパラメータの番号は書き換えたくないので1個目の要素はそのまま渡して動的なパラメータは末尾に追加する
				// 例:
				// 	クエリ:
				//	  SELECT * FROM foo WHERE a = @a AND id IN (sqlc.slice(ids)) AND b = @b
				//  コンパイル済み:
				//    SELECT id, a, b FROM foo WHERE a = ?1 AND id IN (/*SLICE:ids*/?) AND b = ?3
				//  実行時(idsが長さ3の場合):
				//    SELECT id, a, b FROM foo WHERE a = ?1 AND id IN (?2, ?4, ?5) AND b = ?3
				fmt.Fprintf(querier, "  let query = %s;\n", naming.toConstQueryName(q))
				fmt.Fprintf(querier, "  const params: any[] = [%s];\n", buildBindArgs(q))
				for _, p := range q.GetParams() {
					c := p.GetColumn()
					if !c.GetIsSqlcSlice() {
						continue
					}
					n := p.GetNumber()
					propName := naming.toPropertyName(c)
					// sqlc.slice は (/*SLICE:foo*/?) という形式でクエリが書き出される (kyleconroy/sqlc/pull/2274)
					// (?1, ?2, ?3) のような形で書き換える
					fmt.Fprintf(querier, "  query = query.replace(\"(/*SLICE:%s*/?)\", expandedParam(%d, args.%s.length, params.length));\n", c.Name, n, propName)
					// 1番目の要素は予め params に追加されているのでそれ以降を push する
					fmt.Fprintf(querier, "  params.push(...args.%s.slice(1));\n", propName)
				}
				queryVar = "query"
				bindArgs = "...params"
				requireExpandedParams = true
			} else {
				queryVar = naming.toConstQueryName(q)
				bindArgs = buildBindArgs(q)
			}

			fmt.Fprintf(querier, "  return await d1\n")
			fmt.Fprintf(querier, "    .prepare(%s)\n", queryVar)
			if len(q.GetParams()) > 0 {
				fmt.Fprintf(querier, "    .bind(%s)\n", bindArgs)
			}
			switch q.GetCmd() {
			case ":one":
				fmt.Fprintf(querier, "    .first<%s>()", resultType)
			case ":many":
				fmt.Fprintf(querier, "    .all<%s>()", resultType)
			case ":exec":
				fmt.Fprintf(querier, "    .run()")
			}

			// 內部結果型を使っている場合は結果型に変換する処理を生成する
			if needRawType {
				querier.WriteByte('\n')
				if q.GetCmd() == ":one" {
					fmt.Fprintf(querier, "    .then((raw: %s) => raw ? {\n", resultType)
					writeFromRawMapping(querier, "      ", tableMap, q)
					fmt.Fprintf(querier, "    } : null)")
				} else {
					fmt.Fprintf(querier, "    .then((r: D1Result<%s>) => { return {\n", resultType)
					fmt.Fprintf(querier, "      ...r,\n")
					fmt.Fprintf(querier, "      results: r.results ? r.results.map((raw: %s) => { return {\n", resultType)
					writeFromRawMapping(querier, "        ", tableMap, q)
					fmt.Fprintf(querier, "      }}) : undefined,\n")
					fmt.Fprintf(querier, "    }})")
				}
			}
			querier.WriteString(";\n")
			querier.WriteString("}\n")

			querier.WriteByte('\n')
		}

		if requireExpandedParams {
			// sqlc.slice は実行時にクエリ書き換えが必要でそこで使う関数
			querier.WriteString(`function expandedParam(n: number, len: number, last: number): string {
  const params: number[] = [n];
  for (let i = 1; i < len; i++) {
    params.push(last + i);
  }
  return "(" + params.map((x: number) => "?" + x).join(", ") + ")";
}
`)
		}

		if len(requireModels) > 0 {
			var models []string
			for k := range requireModels {
				models = append(models, k)
			}
			sort.Strings(models)
			fmt.Fprintf(header, "import { %s } from \"./models\"\n", strings.Join(models, ", "))
		}
		if header.Len() > 0 {
			header.WriteString("\n")
		}
		files = append(files, &plugin.File{Name: "querier.ts", Contents: append(header.Bytes(), querier.Bytes()...)})
	}

	return &plugin.CodeGenResponse{
		Files: files,
	}, nil
}

// TableMap はスキーマのテーブルの情報を検索可能なマップ
type TableMap struct {
	m map[string]*tableMapEntry
}

func (m *TableMap) findColumn(c *plugin.Column) *plugin.Column {
	t := c.GetTable()
	if t == nil {
		return nil
	}
	table := m.m[t.GetName()]
	if table == nil {
		return nil
	}
	return table.m[c.GetName()]
}

func (m *TableMap) findTable(table *plugin.Identifier) *plugin.Table {
	t := m.m[table.GetName()]
	if t == nil {
		return nil
	}
	return t.t
}

type tableMapEntry struct {
	t *plugin.Table
	m map[string]*plugin.Column
}

func buildTableMap(catalog *plugin.Catalog) TableMap {
	tm := TableMap{
		m: map[string]*tableMapEntry{},
	}
	for _, schema := range catalog.GetSchemas() {
		for _, table := range schema.GetTables() {
			e := tableMapEntry{
				t: table,
				m: map[string]*plugin.Column{},
			}
			for _, column := range table.GetColumns() {
				e.m[column.GetName()] = column
			}
			tm.m[table.GetRel().GetName()] = &e
		}
	}
	return tm
}

// parseOption はクオートされたカンマ区切りの`key=value`形式の入力を受け取りマップとして返す
// 例: `"foo1=bar,foo2=buz"` => map[string]string{"foo1": "bar", "foo2": "buz"}
func parseOption(opt []byte) map[string]string {
	m := map[string]string{}
	if len(opt) == 0 {
		return m
	}
	s, _ := strconv.Unquote(string(opt))
	for _, kv := range strings.Split(s, ",") {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

type TsTypeMap struct {
	m map[string]string
}

func (t *TsTypeMap) toTsType(col *plugin.Column) string {
	dbType := col.GetType().GetName()
	tsType := t.m[dbType]
	if col.GetIsSqlcSlice() {
		tsType += "[]"
	}
	if !col.GetNotNull() {
		tsType += " | null"
	}
	return tsType
}

func buildTsTypeMap(settings *plugin.Settings) *TsTypeMap {
	m := map[string]string{
		"INTEGER":  "number",
		"TEXT":     "string",
		"DATETIME": "string",
		"JSON":     "string",
	}
	for _, o := range settings.GetOverrides() {
		m[o.GetDbType()] = o.GetCodeType()
	}
	return &TsTypeMap{m: m}
}

func toUpperCamel(snake string) string {
	var b strings.Builder
	for _, t := range strings.Split(snake, "_") {
		b.WriteString(strings.ToUpper(t[:1]) + t[1:])
	}
	return b.String()
}

func toLowerCamel(snake string) string {
	s := toUpperCamel(snake)
	return strings.ToLower(s[:1]) + s[1:]
}

type Naming struct{}

// toModelTypeName は models.ts に出力されるモデルの型名を返す
func (Naming) toModelTypeName(table *plugin.Identifier) string {
	return toUpperCamel(table.GetName())
}

// toPropertyName は TypeScript のプロパティの名前を返す
func (Naming) toPropertyName(col *plugin.Column) string {
	return toLowerCamel(col.GetName())
}

// toConstQueryName はクエリ文字列の定数の名前を返す
func (Naming) toConstQueryName(q *plugin.Query) string {
	return toLowerCamel(q.GetName()) + "Query"
}

// toParamsTypeName はクエリのパラメータ型の名前を返す
func (Naming) toParamsTypeName(q *plugin.Query) string {
	return q.GetName() + "Params"
}

// toQueryRowTypeName はクエリの結果型の名前を返す
func (Naming) toQueryRowTypeName(q *plugin.Query) string {
	return q.GetName() + "Row"
}

// toRawQueryRowTypeName はクエリの内部結果型の名前を返す
func (Naming) toRawQueryRowTypeName(q *plugin.Query) string {
	return "Raw" + q.GetName() + "Row"
}

// toEmbedColumnName は sqlc_embed が使われたときのカラム名を返す
func (Naming) toEmbedColumnName(e *plugin.Identifier, c *plugin.Column) string {
	// MEMO: "_" 1つだと最悪他のカラム名と衝突してしまいそう
	return e.GetName() + "_" + c.GetName()
}

// toFunctionName はクエリ関数の関数名を返す
func (Naming) toFunctionName(q *plugin.Query) string {
	return toLowerCamel(q.GetName())
}

var naming Naming

func hasSqlcSlice(q *plugin.Query) bool {
	for _, p := range q.GetParams() {
		if p.GetColumn().GetIsSqlcSlice() {
			return true
		}
	}
	return false
}

func buildBindArgs(q *plugin.Query) string {
	var args strings.Builder
	for i, p := range q.GetParams() {
		if i > 0 {
			args.WriteString(", ")
		}
		args.WriteString("args." + naming.toPropertyName(p.GetColumn()))
		if p.GetColumn().GetIsSqlcSlice() {
			args.WriteString("[0]")
		}
	}
	return args.String()
}

func writeFromRawMapping(w *bytes.Buffer, indent string, tableMap TableMap, q *plugin.Query) {
	for _, c := range q.GetColumns() {
		propName := naming.toPropertyName(c)
		// sqlc_embed の場合はモデル型に変換する
		if et := c.GetEmbedTable(); et.GetName() != "" {
			fmt.Fprintf(w, "%s%s: {\n", indent, propName)
			for _, ec := range tableMap.findTable(et).GetColumns() {
				from := naming.toEmbedColumnName(et, ec)
				to := naming.toPropertyName(ec)
				fmt.Fprintf(w, "%s  %s: raw.%s,\n", indent, to, from)
			}
			fmt.Fprintf(w, "%s},\n", indent)
		} else {
			from := c.GetName()
			fmt.Fprintf(w, "%s%s: raw.%s,\n", indent, propName, from)
		}
	}
}

type Handler func(*plugin.CodeGenRequest) (*plugin.CodeGenResponse, error)

func run(h Handler) error {
	var req plugin.CodeGenRequest
	reqBlob, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	if err := req.UnmarshalVT(reqBlob); err != nil {
		return err
	}
	resp, err := h(&req)
	if err != nil {
		return err
	}
	respBlob, err := resp.MarshalVT()
	if err != nil {
		return err
	}
	w := bufio.NewWriter(os.Stdout)
	if _, err := w.Write(respBlob); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return nil
}

func main() {
	if err := run(handler); err != nil {
		fmt.Fprintf(os.Stderr, "error generating output: %s", err)
		os.Exit(2)
	}
}
