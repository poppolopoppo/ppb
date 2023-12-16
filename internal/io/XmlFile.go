package io

import (
	"fmt"
	"io"

	"github.com/poppolopoppo/ppb/internal/base"
)

type XmlAttr struct {
	Name  string
	Value string
}

func (x XmlAttr) String() string {
	return fmt.Sprint(x.Name, "=\"", x.Value, "\"")
}

type XmlFile struct {
	*base.StructuredFile
}

func NewXmlFile(dst io.Writer, minify bool) *XmlFile {
	return &XmlFile{
		StructuredFile: base.NewStructuredFile(dst, base.STRUCTUREDFILE_DEFAULT_TAB, minify),
	}
}

func (xml *XmlFile) Comment(text string, a ...interface{}) *XmlFile {
	if !xml.Minify() {
		xml.Println(fmt.Sprint("<!-- ", text, " -->"), a...)
	}
	return xml
}
func (xml *XmlFile) Tag(name string, closure func(), attributes ...XmlAttr) *XmlFile {
	if len(attributes) > 0 {
		xml.Print("<%s %s", name, base.JoinString(" ", attributes...))
	} else {
		xml.Print("<%s", name)
	}
	if closure != nil {
		xml.Println(">")
		xml.ScopeIndent(closure)
		xml.Println("</%s>", name)
	} else {
		xml.Println("/>")
	}
	return xml
}
func (xml *XmlFile) InnerString(name, value string, attributes ...XmlAttr) *XmlFile {
	if len(value) > 0 {
		if len(attributes) > 0 {
			xml.Println("<%s %s>%s</%s>", name, base.JoinString(" ", attributes...), value, name)
		} else {
			xml.Println("<%s>%s</%s>", name, value, name)
		}

	}
	return xml
}
