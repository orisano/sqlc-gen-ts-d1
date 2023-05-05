package main

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tabbed/sqlc-go/codegen"
)

type TableMap struct {
	m map[string]*ColumnMap
}

func (m *TableMap) find(c *codegen.Column) *codegen.Column {
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
	m map[string]*codegen.Column
}

func buildTableMap(catalog *codegen.Catalog) TableMap {
	tm := TableMap{
		m: map[string]*ColumnMap{},
	}
	for _, schema := range catalog.GetSchemas() {
		for _, table := range schema.GetTables() {
			cm := ColumnMap{
				m: map[string]*codegen.Column{},
			}
			for _, column := range table.GetColumns() {
				cm.m[column.GetName()] = column
			}
			tm.m[table.GetRel().GetName()] = &cm
		}
	}
	return tm
}

func toLowerCamel(snake string) string {
	tokens := strings.Split(snake, "_")

	var b strings.Builder
	b.WriteString(tokens[0])
	for _, t := range tokens[1:] {
		b.WriteString(strings.ToUpper(t[:1]) + t[1:])
	}
	return b.String()
}

func handler(ctx context.Context, request *codegen.Request) (*codegen.Response, error) {
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

	var files []*codegen.File
	tsTypeMap := map[string]string{
		"INTEGER": "number",
		"TEXT":    "string",
	}
	for _, o := range request.GetSettings().GetOverrides() {
		tsTypeMap[o.GetDbType()] = o.GetCodeType()
	}

	{
		querier := bytes.NewBuffer(nil)

		tableMap := buildTableMap(request.GetCatalog())

		workersTypesPackage := "@cloudflare/workers-types"
		if workersTypesVersion != "" {
			workersTypesPackage += "/" + workersTypesVersion
		}

		if !workersTypesV3 {
			querier.WriteString("import {D1Database, D1Result} from \"" + workersTypesPackage + "\"\n\n")
		}

		for _, q := range request.GetQueries() {
			name := q.GetName()
			lowerName := strings.ToLower(name[:1]) + name[1:]

			query := "-- name: " + q.GetName() + " " + q.GetCmd() + "\n" + q.GetText()
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
					sqliteType := c.GetType().GetName()
					tsType := tsTypeMap[sqliteType]
					if !c.GetNotNull() {
						tsType += " | null"
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
					sqliteType := c.GetType().GetName()
					tsType := tsTypeMap[sqliteType]
					if !c.GetNotNull() {
						tsType += " | null"
					}
					fmt.Fprintf(querier, "  %s: %s;\n", colName, tsType)
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
						fmt.Fprintf(querier, "      %s: raw.%s,\n", to, from)
					}
					fmt.Fprintf(querier, "    } : null)")
				} else {
					fmt.Fprintf(querier, "    .then((r: D1Result<%s>) => { return {\n", resultType)
					fmt.Fprintf(querier, "      ...r,\n")
					fmt.Fprintf(querier, "      results: r.results ? r.results.map((raw: %s) => { return {\n", resultType)
					for _, c := range q.GetColumns() {
						from := c.GetName()
						to := toLowerCamel(from)
						fmt.Fprintf(querier, "        %s: raw.%s,\n", to, from)
					}
					fmt.Fprintf(querier, "      }}) : undefined,\n")
					fmt.Fprintf(querier, "    }})")
				}
			}
			querier.WriteString(";\n")
			querier.WriteString("}\n")

			querier.WriteByte('\n')
		}
		files = append(files, &codegen.File{Name: "querier.ts", Contents: querier.Bytes()})
	}

	return &codegen.Response{
		Files: files,
	}, nil
}

func main() {
	codegen.Run(handler)
}
