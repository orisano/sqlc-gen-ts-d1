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

type TableMap struct {
	m map[string]*ColumnMap
}

func (m *TableMap) find(c *plugin.Column) *plugin.Column {
	t := c.GetTable()
	if t == nil {
		return nil
	}
	cm := m.m[t.GetName()]
	if cm == nil {
		return nil
	}
	return cm.m[c.GetName()]
}

type ColumnMap struct {
	t *plugin.Table
	m map[string]*plugin.Column
}

func buildTableMap(catalog *plugin.Catalog) TableMap {
	tm := TableMap{
		m: map[string]*ColumnMap{},
	}
	for _, schema := range catalog.GetSchemas() {
		for _, table := range schema.GetTables() {
			cm := ColumnMap{
				t: table,
				m: map[string]*plugin.Column{},
			}
			for _, column := range table.GetColumns() {
				cm.m[column.GetName()] = column
			}
			tm.m[table.GetRel().GetName()] = &cm
		}
	}
	return tm
}

func toUpperCamel(snake string) string {
	tokens := strings.Split(snake, "_")
	var b strings.Builder
	for _, t := range tokens {
		b.WriteString(strings.ToUpper(t[:1]) + t[1:])
	}
	return b.String()
}

func toLowerCamel(snake string) string {
	s := toUpperCamel(snake)
	return strings.ToLower(s[:1]) + s[1:]
}

func handler(request *plugin.CodeGenRequest) (*plugin.CodeGenResponse, error) {
	options := map[string]string{}
	if pOpt := string(request.GetPluginOptions()); len(pOpt) > 0 {
		s, _ := strconv.Unquote(pOpt)
		for _, kv := range strings.Split(s, ",") {
			k, v, _ := strings.Cut(kv, "=")
			options[k] = v
		}
	}
	workersTypesVersion := "2022-11-30"
	if v, ok := options["workers-types"]; ok {
		workersTypesVersion = v
	}
	workersTypesV3 := false
	if v, ok := options["workers-types-v3"]; ok {
		workersTypesV3 = v == "1"
	}

	var files []*plugin.File
	tsTypeMap := map[string]string{
		"INTEGER":  "number",
		"TEXT":     "string",
		"DATETIME": "string",
		"JSON":     "string",
	}
	for _, o := range request.GetSettings().GetOverrides() {
		tsTypeMap[o.GetDbType()] = o.GetCodeType()
	}

	{
		models := bytes.NewBuffer(nil)
		for _, s := range request.GetCatalog().GetSchemas() {
			for _, t := range s.GetTables() {
				modelName := toUpperCamel(t.GetRel().GetName())
				fmt.Fprintf(models, "export type %s = {\n", modelName)
				for _, c := range t.GetColumns() {
					colName := toLowerCamel(c.GetName())
					sqliteType := c.GetType().GetName()
					tsType := tsTypeMap[sqliteType]
					if !c.GetNotNull() {
						tsType += " | null"
					}
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
		imports := map[string]bool{}

		const embedSep = "_"

		for _, q := range request.GetQueries() {
			name := q.GetName()
			lowerName := strings.ToLower(name[:1]) + name[1:]

			queryText := q.GetText()
			for _, c := range q.GetColumns() {
				e := c.GetEmbedTable().GetName()
				if e == "" {
					continue
				}
				var news, olds []string
				for _, tc := range tableMap.m[e].t.GetColumns() {
					from := e + "." + tc.GetName()
					to := from + " AS " + e + embedSep + tc.GetName()
					olds = append(olds, from)
					news = append(news, to)
				}
				queryText = strings.Replace(queryText, strings.Join(olds, ", "), strings.Join(news, ", "), 1)
			}

			query := "-- name: " + q.GetName() + " " + q.GetCmd() + "\n" + queryText
			fmt.Fprintf(querier, "const %sQuery = `%s`;\n", lowerName, query)

			querier.WriteByte('\n')

			if len(q.GetParams()) > 0 {
				fmt.Fprintf(querier, "export type %sParams = {\n", name)
				for _, p := range q.GetParams() {
					paramName := toLowerCamel(p.GetColumn().GetName())
					sqliteType := p.GetColumn().GetType().GetName()
					tsType := tsTypeMap[sqliteType]
					nullable := false
					if c := p.GetColumn(); !c.GetNotNull() {
						nullable = true
					} else if tc := tableMap.find(c); tc != nil && !tc.GetNotNull() {
						nullable = true
					}
					if nullable {
						tsType += " | null"
					}
					fmt.Fprintf(querier, "  %s: %s;\n", paramName, tsType)
				}
				querier.WriteString("};\n")

				querier.WriteByte('\n')
			}

			needRawType := false
			rowType := name + "Row"

			if q.GetCmd() != ":exec" {
				fmt.Fprintf(querier, "export type %s = {\n", rowType)
				for _, c := range q.GetColumns() {
					originalColName := c.GetName()
					colName := toLowerCamel(originalColName)
					if originalColName != colName {
						needRawType = true
					}
					tsType := ""
					if sqliteType := c.GetType().GetName(); sqliteType != "" {
						tsType = tsTypeMap[sqliteType]
						if !c.GetNotNull() {
							tsType += " | null"
						}
					} else {
						needRawType = true
						tsType = toUpperCamel(c.GetEmbedTable().GetName())
						imports[tsType] = true
					}
					fmt.Fprintf(querier, "  %s: %s;\n", colName, tsType)
				}
				querier.WriteString("};\n")

				querier.WriteByte('\n')
			}

			if needRawType {
				fmt.Fprintf(querier, "type Raw%s = {\n", rowType)
				for _, c := range q.GetColumns() {
					colName := c.GetName()
					if sqliteType := c.GetType().GetName(); sqliteType != "" {
						tsType := tsTypeMap[sqliteType]
						if !c.GetNotNull() {
							tsType += " | null"
						}
						fmt.Fprintf(querier, "  %s: %s;\n", colName, tsType)
					} else {
						et := c.GetEmbedTable().GetName()
						t := tableMap.m[et]
						for _, tc := range t.t.GetColumns() {
							colName := et + embedSep + tc.GetName()
							sqliteType := tc.GetType().GetName()
							tsType := tsTypeMap[sqliteType]
							if !tc.GetNotNull() {
								tsType += " | null"
							}
							fmt.Fprintf(querier, "  %s: %s;\n", colName, tsType)
						}
					}
				}
				querier.WriteString("};\n")

				querier.WriteByte('\n')
			}

			var retType, resultType string
			if cmd := q.GetCmd(); cmd == ":one" {
				retType = rowType + " | null"
				resultType = retType
				if needRawType {
					resultType = "Raw" + rowType + " | null"
				}
			} else if cmd == ":exec" {
				retType = "D1Result"
			} else {
				retType = "D1Result<" + rowType + ">"
				resultType = rowType
				if needRawType {
					resultType = "Raw" + rowType
				}
			}

			fmt.Fprintf(querier, "export async function %s(\n", lowerName)
			fmt.Fprintf(querier, "  d1: D1Database")
			if len(q.GetParams()) > 0 {
				querier.WriteString(",\n")
				fmt.Fprintf(querier, "  args: %sParams", name)
			}
			querier.WriteString("\n")
			fmt.Fprintf(querier, "): Promise<%s> {\n", retType)
			fmt.Fprintf(querier, "  return await d1\n")
			fmt.Fprintf(querier, "    .prepare(%sQuery)\n", lowerName)
			if len(q.GetParams()) > 0 {
				querier.WriteString("    .bind(")
				for i, p := range q.GetParams() {
					if i > 0 {
						querier.WriteString(", ")
					}
					querier.WriteString("args." + toLowerCamel(p.GetColumn().GetName()))
				}
				querier.WriteString(")\n")
			}
			switch q.GetCmd() {
			case ":one":
				fmt.Fprintf(querier, "    .first<%s>()", resultType)
			case ":many":
				fmt.Fprintf(querier, "    .all<%s>()", resultType)
			case ":exec":
				fmt.Fprintf(querier, "    .run()")
			}
			if needRawType {
				querier.WriteByte('\n')

				if q.GetCmd() == ":one" {
					fmt.Fprintf(querier, "    .then((raw: %s) => raw ? {\n", resultType)
					for _, c := range q.GetColumns() {
						from := c.GetName()
						to := toLowerCamel(from)
						if et := c.GetEmbedTable().GetName(); et != "" {
							fmt.Fprintf(querier, "      %s: {\n", to)
							for _, tc := range tableMap.m[et].t.GetColumns() {
								from := et + embedSep + tc.GetName()
								to := toLowerCamel(tc.GetName())
								fmt.Fprintf(querier, "        %s: raw.%s,\n", to, from)
							}
							fmt.Fprintf(querier, "      },\n")
						} else {
							fmt.Fprintf(querier, "      %s: raw.%s,\n", to, from)
						}
					}
					fmt.Fprintf(querier, "    } : null)")
				} else {
					fmt.Fprintf(querier, "    .then((r: D1Result<%s>) => { return {\n", resultType)
					fmt.Fprintf(querier, "      ...r,\n")
					fmt.Fprintf(querier, "      results: r.results ? r.results.map((raw: %s) => { return {\n", resultType)
					for _, c := range q.GetColumns() {
						from := c.GetName()
						to := toLowerCamel(from)
						if et := c.GetEmbedTable().GetName(); et != "" {
							fmt.Fprintf(querier, "        %s: {\n", to)
							for _, tc := range tableMap.m[et].t.GetColumns() {
								from := et + embedSep + tc.GetName()
								to := toLowerCamel(tc.GetName())
								fmt.Fprintf(querier, "          %s: raw.%s,\n", to, from)
							}
							fmt.Fprintf(querier, "        },\n")
						} else {
							fmt.Fprintf(querier, "        %s: raw.%s,\n", to, from)
						}
					}
					fmt.Fprintf(querier, "      }}) : undefined,\n")
					fmt.Fprintf(querier, "    }})")
				}
			}
			querier.WriteString(";\n")
			querier.WriteString("}\n")

			querier.WriteByte('\n')
		}
		if len(imports) > 0 {
			var models []string
			for k := range imports {
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
