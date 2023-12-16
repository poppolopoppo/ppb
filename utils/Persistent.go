package utils

import (
	"flag"
	"fmt"
	"github.com/poppolopoppo/ppb/internal/base"
	"io"
)

var LogPersistent = base.NewLogCategory("Persistent")

type PersistentVar interface {
	fmt.Stringer
	flag.Value
	base.Serializable
}

type BoolVar = base.InheritableBool
type IntVar = base.InheritableInt
type BigIntVar = base.InheritableBigInt
type StringVar = base.InheritableString

type PersistentData interface {
	PinData() map[string]string
	LoadData(object string, property string, value PersistentVar) error
	StoreData(object string, property string, value PersistentVar)
}

type persistentData struct {
	Data map[string]map[string]string
}

func NewPersistentMap(name string) *persistentData {
	return &persistentData{
		Data: make(map[string]map[string]string),
	}
}
func (pmp *persistentData) Len() (result int) {
	for _, vars := range pmp.Data {
		result += len(vars)
	}
	return
}
func (pmp *persistentData) PinData() (result map[string]string) {
	result = make(map[string]string, len(pmp.Data))
	for object, it := range pmp.Data {
		for property, value := range it {
			result[fmt.Sprint(object, `.`, property)] = value
		}
	}
	return
}
func (pmp *persistentData) LoadData(name string, property string, dst PersistentVar) error {
	if object, ok := pmp.Data[name]; ok {
		if value, ok := object[property]; ok {
			base.LogDebug(LogPersistent, "load object property %s.%s = %v", name, property, value)
			return dst.Set(value)
		} else {
			err := fmt.Errorf("object %q has no property %q", name, property)
			base.LogWarningVerbose(LogPersistent, "load(%s.%s): %v", name, property, err)
			return err
		}

	} else {
		err := fmt.Errorf("object '%s' not found", name)
		base.LogWarningVerbose(LogPersistent, "load(%s.%s): %v", name, property, err)
		return err
	}
}
func (pmp *persistentData) StoreData(name string, property string, dst PersistentVar) {
	base.LogDebug(LogPersistent, "store in %s.%s = %v", name, property, dst)
	object, ok := pmp.Data[name]
	if !ok {
		object = make(map[string]string)
		pmp.Data[name] = object
	}
	object[property] = dst.String()
}
func (pmp *persistentData) Serialize(dst io.Writer) error {
	if err := base.JsonSerialize(&pmp.Data, dst, base.OptionJsonPrettyPrint(true)); err == nil {
		base.LogDebug(LogPersistent, "saved %d vars from config to disk", pmp.Len())
		return nil
	} else {
		return fmt.Errorf("failed to serialize config: %v", err)
	}
}
func (pmp *persistentData) Deserialize(src io.Reader) error {
	if err := base.JsonDeserialize(&pmp.Data, src); err == nil {
		base.LogVerbose(LogPersistent, "loaded %d vars from disk to config", pmp.Len())
		return nil
	} else {
		return fmt.Errorf("failed to deserialize config: %v", err)
	}
}
