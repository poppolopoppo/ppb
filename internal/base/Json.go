package base

import (
	"bytes"
	"encoding"
	"flag"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unsafe"

	jsonSlow "encoding/json"

	jsonFast "github.com/goccy/go-json"

	jsonSchema "github.com/invopop/jsonschema"
)

type JsonMap map[string]interface{}

/***************************************
 * JSON Marshalling for formattable elements
 ***************************************/

func MarshalJSON[T fmt.Stringer](x T) ([]byte, error) {
	return jsonFast.Marshal(x.String())
}
func UnmarshalJSON[T flag.Value](x T, data []byte) error {
	var str string
	if err := jsonFast.Unmarshal(data, &str); err != nil {
		return err
	}
	return x.Set(str)
}

/***************************************
 * JSON Serialization
 ***************************************/

type JsonOptions struct {
	PrettyPrint bool
}

type JsonOptionFunc = func(*JsonOptions)

func OptionJsonPrettyPrint(enabled bool) JsonOptionFunc {
	return func(jo *JsonOptions) {
		jo.PrettyPrint = enabled
	}
}

func JsonSerialize(x interface{}, dst io.Writer, options ...JsonOptionFunc) error {
	var opts JsonOptions
	for _, it := range options {
		it(&opts)
	}

	encoder := jsonFast.NewEncoder(dst)

	if opts.PrettyPrint {
		encoder.SetIndent("", "  ")
	} else {
		encoder.SetIndent("", "")
	}

	return encoder.EncodeWithOption(x,
		jsonFast.UnorderedMap(),
		jsonFast.DisableHTMLEscape(),
		jsonFast.DisableNormalizeUTF8())
}
func JsonDeserialize(x interface{}, src io.Reader) error {
	decoder := jsonFast.NewDecoder(src)

	// we want errors by default when unknown fields are found in json file
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(x); err == nil {
		return nil
	} else {
		return err
	}
}

/***************************************
 * Generate a JSON schema from an object
 ***************************************/

// More conservative than the draft-20 schema version (for VSCode compatibility)
const jsonSchemaVersion = "http://json-schema.org/draft-07/schema#"

var interface_autocompletable = reflect.TypeFor[AutoCompletable]()
var interface_textMarshaller = reflect.TypeFor[encoding.TextMarshaler]()
var interface_enumGettable = reflect.TypeFor[EnumGettable]()

type customJsonSchemaType struct {
	reflector         *jsonSchema.Reflector
	autocompleteParam any
	autocompletables  map[reflect.Type]*jsonSchema.Schema
	enumSets          map[reflect.Type]*jsonSchema.Schema
	maps              map[reflect.Type]*jsonSchema.Schema
	textMarshallers   map[reflect.Type]*jsonSchema.Schema
}

func newCustomJsonSchemaType(reflector *jsonSchema.Reflector, autocompleteParam any) customJsonSchemaType {
	return customJsonSchemaType{
		reflector:         reflector,
		autocompleteParam: autocompleteParam,
		autocompletables:  make(map[reflect.Type]*jsonSchema.Schema, 32),
		enumSets:          make(map[reflect.Type]*jsonSchema.Schema, 32),
		maps:              make(map[reflect.Type]*jsonSchema.Schema, 32),
		textMarshallers:   make(map[reflect.Type]*jsonSchema.Schema, 32),
	}
}

func (x *customJsonSchemaType) createRefSchema(def *jsonSchema.Schema) *jsonSchema.Schema {
	return &jsonSchema.Schema{
		Ref: def.ID.String(), // e.g. "#/$defs/MyDef"
	}
}

func (x *customJsonSchemaType) createArraySchema(items *jsonSchema.Schema) *jsonSchema.Schema {
	return &jsonSchema.Schema{
		Type:  "array",
		Items: items,
	}
}

func (x *customJsonSchemaType) createAutocompletableSchema(t reflect.Type) (schema *jsonSchema.Schema) {
	var ok bool
	if schema, ok = x.autocompletables[t]; !ok {
		var defaultValue = reflect.New(t)
		allowedValues := GatherAutoCompletionFrom(defaultValue.Interface(), x.autocompleteParam)

		description := strings.Builder{}
		for i, it := range allowedValues {
			fmt.Fprintf(&description, "%s%s: %s", Blend("\n", "", i == 0), it.Text, it.Description)
		}

		schema = &jsonSchema.Schema{
			Type: "string",
			Enum: Map(func(ac AutoCompleteResult) any {
				return ac.Text
			}, allowedValues...),
			Description: description.String(),
		}

		if isItemsSet(t) {
			// if the type is a set, we need to create an array schema
			elem := schema
			elem.ID = jsonSchema.ID("").Def(x.namer(t.Elem()))
			schema = x.createArraySchema(elem)
		}

		schema.ID = jsonSchema.ID("").Def(x.namer(t))

		x.autocompletables[t] = schema
	}
	return
}

func (x *customJsonSchemaType) createEnumSetSchema(t reflect.Type) (schema *jsonSchema.Schema) {
	var ok bool
	if schema, ok = x.enumSets[t]; !ok {
		var defaultValue = reflect.New(t).Interface().(EnumSettable)
		allowedValues := GatherAutoCompletionFrom(defaultValue, x.autocompleteParam)

		if sliceFn, ok := t.MethodByName("Slice"); ok {
			var enumValue int32
			defaultValue.FromOrd(reflect.NewAt(sliceFn.Type.Out(0).Elem(), unsafe.Pointer(&enumValue)).Interface().(EnumFlag).Mask())
		} else {
			LogPanic(LogBase, "EnumSet type %s does not implement Slice method", t.Name())
		}

		allowedPattern := defaultValue.String()

		schema = &jsonSchema.Schema{
			Type:    "string",
			Pattern: fmt.Sprintf(`^(?:%s)(?:\|(?:%s))*$`, allowedPattern, allowedPattern),
			Enum: Map(func(ac AutoCompleteResult) any {
				return ac.Text
			}, allowedValues...),
		}

		schema.ID = jsonSchema.ID("").Def(x.namer(t))

		x.enumSets[t] = schema
	}
	return
}

func (x *customJsonSchemaType) createMapSchema(t reflect.Type) (schema *jsonSchema.Schema) {
	var ok bool
	if schema, ok = x.maps[t]; !ok {
		schema = &jsonSchema.Schema{
			Type: "object",
		}

		key := x.createSchemaForType(t.Key())
		value := x.mapper(t.Elem())
		if value == nil {
			value = &jsonSchema.Schema{
				Ref: jsonSchema.ID("").Def(x.namer(t.Elem())).String(), // e.g. "#/$defs/MyDef"
			}
		}

		schema.Properties = jsonSchema.NewProperties()
		for _, it := range key.Enum {
			schema.Properties.Set(it.(string), value)
		}

		schema.ID = jsonSchema.ID("").Def(x.namer(t))

		x.maps[t] = schema
	}
	return
}

func (x *customJsonSchemaType) createTextMarshallerSchema(t reflect.Type) (schema *jsonSchema.Schema) {
	var ok bool
	if schema, ok = x.textMarshallers[t]; !ok {
		schema = &jsonSchema.Schema{
			Type: "string",
		}

		switch x.namer(t) {
		case "Filename":
			schema.Description = "A file path, relative to the root of the project."
			schema.Pattern = `^[a-zA-Z0-9_\-\.\/\\]+$`
		case "Directory":
			schema.Description = "A directory path, relative to the root of the project."
			schema.Pattern = `^[a-zA-Z0-9_\-\.\/\\]+$`
		}

		schema.ID = jsonSchema.ID("").Def(x.namer(t))

		x.textMarshallers[t] = schema
	}
	return
}

func isItemsSet(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Array || t.Kind() == reflect.Slice {
		return true
	}
	return false
}

func (x *customJsonSchemaType) namer(t reflect.Type) string {
	if isItemsSet(t) {
		return t.Elem().Name() + "Set"
	}
	if t.Implements(interface_enumGettable) {
		if sliceFn, ok := t.MethodByName("Slice"); ok {
			return sliceFn.Type.Out(0).Elem().Name() + "Flags"
		} else {
			LogPanic(LogBase, "EnumSet type %s does not implement Slice method", t.Name())
		}
	}
	if t.Kind() == reflect.Map {
		return fmt.Sprintf("Map_%s_%s", x.namer(t.Key()), x.namer(t.Elem()))
	}
	return t.Name()
}

func (x *customJsonSchemaType) createSchemaForType(t reflect.Type) (schema *jsonSchema.Schema) {
	if t.Implements(interface_enumGettable) {
		schema = x.createEnumSetSchema(t)
	} else if t.Implements(interface_autocompletable) {
		schema = x.createAutocompletableSchema(t)
	} else if t.Implements(interface_textMarshaller) {
		schema = x.createTextMarshallerSchema(t)
	} else if t.Kind() == reflect.Map {
		schema = x.createMapSchema(t)
	}
	return
}

func (x *customJsonSchemaType) mapper(t reflect.Type) *jsonSchema.Schema {
	if schema := x.createSchemaForType(t); schema != nil {
		return x.createRefSchema(schema)
	}
	// use default behavior for all other types
	return nil
}

func (x *customJsonSchemaType) finalize(schema *jsonSchema.Schema) {
	schema.Version = jsonSchemaVersion

	// finalize the schema by setting the definitions
	if len(x.autocompletables) > 0 {
		if schema.Definitions == nil {
			schema.Definitions = make(map[string]*jsonSchema.Schema, len(x.autocompletables))
		}
		for t, def := range x.autocompletables {
			schema.Definitions[x.namer(t)] = def
		}
	}

	if len(x.enumSets) > 0 {
		if schema.Definitions == nil {
			schema.Definitions = make(map[string]*jsonSchema.Schema, len(x.enumSets))
		}
		for t, def := range x.enumSets {
			schema.Definitions[x.namer(t)] = def
		}
	}

	if len(x.maps) > 0 {
		if schema.Definitions == nil {
			schema.Definitions = make(map[string]*jsonSchema.Schema, len(x.maps))
		}
		for t, def := range x.maps {
			schema.Definitions[x.namer(t)] = def
		}
	}

	if len(x.textMarshallers) > 0 {
		if schema.Definitions == nil {
			schema.Definitions = make(map[string]*jsonSchema.Schema, len(x.textMarshallers))
		}
		for t, def := range x.textMarshallers {
			schema.Definitions[x.namer(t)] = def
		}
	}
}

func makeJsonSchemaReflector(autoCompleteParam any) (customType customJsonSchemaType) {
	reflector := jsonSchema.Reflector{}
	customType = newCustomJsonSchemaType(&reflector, autoCompleteParam)
	reflector.Mapper = customType.mapper
	reflector.Namer = customType.namer
	return
}

func createJsonSchema(t reflect.Type, autoCompleteParam any) *jsonSchema.Schema {
	r := makeJsonSchemaReflector(autoCompleteParam)
	schema := r.reflector.ReflectFromType(t)
	r.finalize(schema)
	return schema
}

func JsonSchemaFromType(t reflect.Type, autoCompleteParam any, dst io.Writer, options ...JsonOptionFunc) error {
	schema := createJsonSchema(t, autoCompleteParam)
	return JsonSerialize(schema, dst, options...)
}

/***************************************
 * Pretty print an object using Json serialization
 ***************************************/

func PrettyPrint(x interface{}) string {
	tmp := TransientPage64KiB.Allocate()
	defer TransientPage64KiB.Release(tmp)

	buf := bytes.NewBuffer((*tmp)[:0])

	encoder := jsonSlow.NewEncoder(buf)

	var err error
	if err = encoder.Encode(x); err == nil {
		tmp2 := TransientPage64KiB.Allocate()
		defer TransientPage64KiB.Release(tmp2)

		pretty := bytes.NewBuffer((*tmp2)[:0])

		if err = jsonSlow.Indent(pretty, buf.Bytes(), "", "\t"); err == nil {
			return pretty.String()
		}
	}
	return fmt.Sprint(err)
}

type PrettyPrinter struct {
	Ref interface{}
}

func (x PrettyPrinter) String() string {
	return PrettyPrint(x.Ref)
}
