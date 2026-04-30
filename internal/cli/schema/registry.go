package schema

var outputSchemas = map[string]string{}

func Register(commandPath, jsonSchemaLiteral string) {
	outputSchemas[commandPath] = jsonSchemaLiteral
}

func Get(path string) (string, bool) {
	s, ok := outputSchemas[path]
	return s, ok
}

func All() map[string]string {
	out := make(map[string]string, len(outputSchemas))
	for k, v := range outputSchemas {
		out[k] = v
	}
	return out
}
