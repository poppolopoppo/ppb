package base

import (
	"net/url"
	"regexp"
	"strings"
)

/***************************************
 * Serializable Regexp
 ***************************************/

type Regexp struct {
	*regexp.Regexp
}

func NewRegexp(pattern string) Regexp {
	return Regexp{regexp.MustCompile(pattern)}
}
func (x Regexp) Valid() bool { return x.Regexp != nil }
func (x *Regexp) Set(in string) (err error) {
	x.Regexp, err = regexp.Compile(in)
	return
}
func (x *Regexp) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *Regexp) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}
func (x *Regexp) Serialize(ar Archive) {
	if ar.Flags().IsLoading() {
		var url string
		ar.String(&url)
		if err := x.Set(url); err != nil {
			ar.OnError(err)
		}
	} else {
		url := x.String()
		ar.String(&url)
	}
}
func (x Regexp) Concat(patterns ...Regexp) Regexp {
	merged := strings.Builder{}

	var n int
	if x.Valid() {
		n++
		merged.WriteString("(?:") // non-capturing group
		merged.WriteString(x.String())
		merged.WriteRune(')')
	} else if len(patterns) == 0 {
		return Regexp{} // invalid pattern
	}

	for _, it := range patterns {
		if it.Valid() {
			if n > 0 {
				merged.WriteRune('|')
			}
			n++
			merged.WriteString("(?:") // non-capturing group
			merged.WriteString(it.String())
			merged.WriteRune(')')
		}
	}

	if n > 0 {
		return Regexp{Regexp: regexp.MustCompile(merged.String())}
	} else {
		return Regexp{}
	}
}

/***************************************
 * Serializable URL
 ***************************************/

type Url struct {
	*url.URL
}

func (x Url) Valid() bool { return x.URL != nil }
func (x *Url) Set(in string) (err error) {
	x.URL, err = url.Parse(in)
	return
}
func (x *Url) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *Url) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}
func (x *Url) Serialize(ar Archive) {
	if ar.Flags().IsLoading() {
		var url string
		ar.String(&url)
		if err := x.Set(url); err != nil {
			ar.OnError(err)
		}
	} else {
		url := x.String()
		ar.String(&url)
	}
}
