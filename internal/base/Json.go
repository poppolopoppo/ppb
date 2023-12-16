package base

import (
	"bytes"
	"flag"
	"fmt"
	"io"

	slowJson "encoding/json"

	fastJson "github.com/goccy/go-json"
)

type JsonMap map[string]interface{}

/***************************************
 * JSON
 ***************************************/

func MarshalJSON[T fmt.Stringer](x T) ([]byte, error) {
	return fastJson.Marshal(x.String())
}
func UnmarshalJSON[T flag.Value](x T, data []byte) error {
	var str string
	if err := fastJson.Unmarshal(data, &str); err != nil {
		return err
	}
	return x.Set(str)
}

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

	encoder := fastJson.NewEncoder(dst)

	if opts.PrettyPrint {
		encoder.SetIndent("", "  ")
	} else {
		encoder.SetIndent("", "")
	}

	return encoder.EncodeWithOption(x,
		fastJson.UnorderedMap(),
		fastJson.DisableHTMLEscape(),
		fastJson.DisableNormalizeUTF8())
}
func JsonDeserialize(x interface{}, src io.Reader) error {
	decoder := fastJson.NewDecoder(src)
	if err := decoder.Decode(x); err == nil {
		return nil
	} else {
		return err
	}
}

func PrettyPrint(x interface{}) string {
	tmp := TransientPage64KiB.Allocate()
	defer TransientPage64KiB.Release(tmp)

	buf := bytes.NewBuffer(tmp[:0])

	encoder := slowJson.NewEncoder(buf)

	var err error
	if err = encoder.Encode(x); err == nil {
		tmp2 := TransientPage64KiB.Allocate()
		defer TransientPage64KiB.Release(tmp2)

		pretty := bytes.NewBuffer(tmp2[:0])

		if err = slowJson.Indent(pretty, buf.Bytes(), "", "\t"); err == nil {
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
