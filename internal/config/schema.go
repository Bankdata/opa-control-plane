package config

import (
	"encoding/json"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	schemareflector "github.com/swaggest/jsonschema-go"
)

var rootSchema *jsonschema.Schema

func init() {
	reflector := schemareflector.Reflector{}
	reflector.AddTypeMapping(SecretRef{}, "")
	reflector.AddTypeMapping(Selector{}, map[string][]string{})
	s, err := reflector.Reflect(Root{})
	if err != nil {
		panic(err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		panic(err)
	}

	js, err := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
	if err != nil {
		panic(err)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", js); err != nil {
		panic(err)
	}

	rootSchema, err = compiler.Compile("schema.json")
	if err != nil {
		panic(err)
	}
}
