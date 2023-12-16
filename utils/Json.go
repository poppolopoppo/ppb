package utils

import (
	"bytes"
	"flag"
	"fmt"
	"io"

	"github.com/goccy/go-json"

	slowJson "encoding/json"
)

type JsonMap map[string]interface{}

/***************************************
 * JSON
 ***************************************/

func MarshalJSON[T fmt.Stringer](x T) ([]byte, error) {
	return json.Marshal(x.String())
}
func UnmarshalJSON[T flag.Value](x T, data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
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

	encoder := json.NewEncoder(dst)
	encoder.SetIndent("", "  ")

	return encoder.Encode(x)
}
func JsonDeserialize(x interface{}, src io.Reader) error {
	decoder := json.NewDecoder(src)
	if err := decoder.Decode(x); err == nil {
		return nil
	} else {
		return err
	}
}

func PrettyPrint(x interface{}) string {
	tmp := TransientLargePage.Allocate()
	defer TransientLargePage.Release(tmp)

	buf := bytes.NewBuffer(tmp[:0])

	encoder := slowJson.NewEncoder(buf)

	var err error
	if err = encoder.Encode(x); err == nil {
		tmp2 := TransientLargePage.Allocate()
		defer TransientLargePage.Release(tmp2)

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
