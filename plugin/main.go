package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/tabbed/sqlc-go/codegen"
)

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
	var files []*codegen.File
	tsTypeMap := map[string]string{
		"INTEGER": "bigint",
		"TEXT":    "string",
	}
	{
		querier := bytes.NewBuffer(nil)
		for _, q := range request.GetQueries() {
			name := q.GetName()
			lowerName := strings.ToLower(name[:1]) + name[1:]

			fmt.Fprintf(querier, "const %sQuery = `%s`;\n", lowerName, q.GetText())

			querier.WriteByte('\n')

			fmt.Fprintf(querier, "export type %sParams = {\n", name)
			for _, p := range q.GetParams() {
				paramName := toLowerCamel(p.GetColumn().GetName())
				sqliteType := p.GetColumn().GetType().GetName()
				tsType := tsTypeMap[sqliteType]
				fmt.Fprintf(querier, "  %s: %s;\n", paramName, tsType)
			}
			fmt.Fprintf(querier, "};\n")

			querier.WriteByte('\n')

			fmt.Fprintf(querier, "export type %sRow = {\n", name)
			for _, c := range q.GetColumns() {
				colName := c.GetName()
				sqliteType := c.GetType().GetName()
				tsType := tsTypeMap[sqliteType]
				if !c.GetNotNull() {
					tsType += " | null"
				}
				fmt.Fprintf(querier, "  %s: %s;\n", colName, tsType)
			}
			fmt.Fprintf(querier, "};\n")

			querier.WriteByte('\n')

			fmt.Fprintf(querier, "export async function %s(\n", lowerName)
			fmt.Fprintf(querier, "  d1: D1Database,\n")
			fmt.Fprintf(querier, "  args: %sParams\n", name)
			fmt.Fprintf(querier, "): Promise<%sRow | null> {\n", name)
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
				fmt.Fprintf(querier, "    .first<%sRow | null>();\n", name)
			case ":many":
				fmt.Fprintf(querier, "    .all<%sRow[]>();\n", name)
			case ":exec":
				fmt.Fprintf(querier, "    .run<%sRow | null>();\n", name)
			}
			fmt.Fprintf(querier, "}\n")

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
