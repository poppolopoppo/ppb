package base

import (
	"reflect"
	"testing"
	"time"
)

func TestInheritableString(t *testing.T) {
	var s InheritableString
	s.Set("hello")
	if s.Get() != "hello" {
		t.Errorf("expected 'hello', got %s", s.Get())
	}
	s.Set(INHERIT_STRING)
	if !s.IsInheritable() {
		t.Errorf("expected IsInheritable to be true")
	}
	s2 := InheritableString("world")
	s.Inherit(s2)
	if s != s2 {
		t.Errorf("expected inherit to set value to 'world'")
	}
	s.Set("")
	s2 = InheritableString("abc")
	s.Inherit(s2)
	if s != s2 {
		t.Errorf("inherit from empty should set value")
	}
}

func TestInheritableByte(t *testing.T) {
	var b InheritableByte
	b.Set("5")
	if b.Get() != 5 {
		t.Errorf("expected 5, got %d", b.Get())
	}
	b.Set(INHERIT_STRING)
	if !b.IsInheritable() {
		t.Errorf("expected IsInheritable true")
	}
	b2 := InheritableByte(7)

	b.Inherit(b2)
	if b != b2 {
		t.Errorf("inherit did not set value")
	}
	b.Set("0")
	b2 = InheritableByte(9)
	b.Overwrite(b2)
	if b != b2 {
		t.Errorf("overwrite did not set value")
	}
}

func TestInheritableInt(t *testing.T) {
	var i InheritableInt
	i.Set("42")
	if i.Get() != 42 {
		t.Errorf("expected 42, got %d", i.Get())
	}
	i.Set(INHERIT_STRING)
	if !i.IsInheritable() {
		t.Errorf("expected IsInheritable true")
	}
	i2 := InheritableInt(99)
	i.Inherit(i2)
	if i != i2 {
		t.Errorf("inherit did not set value")
	}
	i.Set("0")
	i2 = InheritableInt(123)
	i.Overwrite(i2)
	if i != i2 {
		t.Errorf("overwrite did not set value")
	}
}

func TestInheritableBigInt(t *testing.T) {
	var i InheritableBigInt
	i.Set("1234567890123")
	if i.Get() != 1234567890123 {
		t.Errorf("expected 1234567890123, got %d", i.Get())
	}
	i.Set(INHERIT_STRING)
	if !i.IsInheritable() {
		t.Errorf("expected IsInheritable true")
	}
	i2 := InheritableBigInt(9876543210)
	i.Inherit(i2)
	if i != i2 {
		t.Errorf("inherit did not set value")
	}
	i.Set("0")
	i2 = InheritableBigInt(555)
	i.Overwrite(i2)
	if i != i2 {
		t.Errorf("overwrite did not set value")
	}
}

func TestSizeInBytes_Set(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"100", 100},
		{"1KiB", 1024},
		{"2MiB", 2 * 1024 * 1024},
		{"3GiB", 3 * 1024 * 1024 * 1024},
		{"4TiB", 4 * 1024 * 1024 * 1024 * 1024},
		{"5PiB", 5 * 1024 * 1024 * 1024 * 1024 * 1024},
		{"1KB", 1000},
		{"2MB", 2 * 1000 * 1000},
		{"INHERIT", int64(INHERIT_VALUE)},
	}
	for _, tt := range tests {
		var s SizeInBytes
		if err := s.Set(tt.input); err != nil {
			t.Errorf("Set(%q) error: %v", tt.input, err)
		}
		if s.Get() != tt.expected {
			t.Errorf("Set(%q) = %d, want %d", tt.input, s.Get(), tt.expected)
		}
	}
}

func TestTimespan_Set(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"100", 100 * int64(Second)},
		{"1ms", 1 * int64(Millisecond)},
		{"2s", 2 * int64(Second)},
		{"3m", 3 * int64(Minute)},
		{"4h", 4 * int64(Hour)},
		{"5d", 5 * int64(Day)},
		{"6w", 6 * int64(Week)},
		{"INHERIT", int64(INHERIT_VALUE)},
	}
	for _, tt := range tests {
		var ts Timespan
		if err := ts.Set(tt.input); err != nil {
			t.Errorf("Set(%q) error: %v", tt.input, err)
		}
		if ts.Get() != tt.expected {
			t.Errorf("Set(%q) = %d, want %d", tt.input, ts.Get(), tt.expected)
		}
	}
}

func TestInheritableBool(t *testing.T) {
	var b InheritableBool
	b.Set("TRUE")
	if !b.Get() {
		t.Errorf("expected TRUE")
	}
	b.Set("FALSE")
	if b.Get() {
		t.Errorf("expected FALSE")
	}
	b.Set(INHERIT_STRING)
	if !b.IsInheritable() {
		t.Errorf("expected IsInheritable true")
	}
	b.Enable()
	if !b.Get() {
		t.Errorf("Enable failed")
	}
	b.Disable()
	if b.Get() {
		t.Errorf("Disable failed")
	}
	b.Toggle()
	if !b.Get() {
		t.Errorf("Toggle failed")
	}
}

func TestInheritableSlice(t *testing.T) {
	var _ InheritableSlicable[InheritableString] = InheritableString("abc")
	s := InheritableSlice[InheritableString, *InheritableString]{InheritableString("a"), InheritableString("b")}
	if !reflect.DeepEqual(s.Get(), []InheritableString{"a", "b"}) {
		t.Errorf("Get failed")
	}
	if !s.Equals(InheritableSlice[InheritableString, *InheritableString]{InheritableString("a"), InheritableString("b")}) {
		t.Errorf("Equals failed")
	}
	s2 := InheritableSlice[InheritableString, *InheritableString]{}
	s2.Inherit(s)
	if !reflect.DeepEqual(s2, s) {
		t.Errorf("Inherit failed")
	}
	s2 = InheritableSlice[InheritableString, *InheritableString]{}
	s2.Overwrite(s)
	if !reflect.DeepEqual(s2, s) {
		t.Errorf("Overwrite failed")
	}
}

func TestInheritableCommandLine(t *testing.T) {
	var b InheritableByte
	ok, err := b.CommandLine("b", "-b=5")
	if !ok || err != nil || b.Get() != 5 {
		t.Errorf("CommandLine failed: %v %v %v", ok, err, b.Get())
	}
	var i InheritableInt
	ok, err = i.CommandLine("i", "-i=42")
	if !ok || err != nil || i.Get() != 42 {
		t.Errorf("CommandLine failed: %v %v %v", ok, err, i.Get())
	}
	var s InheritableString
	ok, err = InheritableCommandLine("foo", "-foo=bar", &s)
	if !ok || err != nil || s.Get() != "bar" {
		t.Errorf("InheritableCommandLine failed: %v %v %v", ok, err, s.Get())
	}
}

func TestSizeInBytes_String(t *testing.T) {
	tests := []struct {
		val      SizeInBytes
		expected string
	}{
		{100, "100 b"},
		{1024, "1.00 Kib"},
		{1024 * 1024, "1.00 Mib"},
		{1024 * 1024 * 1024, "1.00 Gib"},
		{1024 * 1024 * 1024 * 1024, "1.00 Tib"},
		{1024 * 1024 * 1024 * 1024 * 1024, "1.00 Pib"},
	}
	for _, tt := range tests {
		if got := tt.val.String(); got != tt.expected {
			t.Errorf("String() = %q, want %q", got, tt.expected)
		}
	}
}

func TestTimespan_String(t *testing.T) {
	tests := []struct {
		val      Timespan
		expected string
	}{
		{500, "500 µs"},
		{1500, "1.50 ms"},
		{2 * Second, "2.00 seconds"},
		{3 * Minute, "3.00 minutes"},
		{4 * Hour, "4.00 hours"},
		{5 * Day, "5.00 days"},
		{6 * Week, "6.00 weeks"},
	}
	for _, tt := range tests {
		if got := tt.val.String(); got != tt.expected {
			t.Errorf("String() = %q, want %q", got, tt.expected)
		}
	}
}

func TestInheritMaxMin(t *testing.T) {
	x := InheritableInt(10)
	y := InheritableInt(20)
	max := InheritMax(x, y)
	if max != y {
		t.Errorf("InheritMax failed: got %v, want %v", max, y)
	}
	min := InheritMin(x, y)
	if min != x {
		t.Errorf("InheritMin failed: got %v, want %v", min, x)
	}
}

func TestSizeInBytes_MarshalUnmarshalText(t *testing.T) {
	var s SizeInBytes
	s.Set("1024")
	data, err := s.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	var s2 SizeInBytes
	if err := s2.UnmarshalText(data); err != nil {
		t.Fatalf("UnmarshalText error: %v", err)
	}
	if s != s2 {
		t.Errorf("Marshal/Unmarshal mismatch: %v vs %v", s, s2)
	}
}

func TestTimespan_MarshalUnmarshalText(t *testing.T) {
	var ts Timespan
	ts.Set("1234")
	data, err := ts.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	var ts2 Timespan
	if err := ts2.UnmarshalText(data); err != nil {
		t.Fatalf("UnmarshalText error: %v", err)
	}
	if ts != ts2 {
		t.Errorf("Marshal/Unmarshal mismatch: %v vs %v", ts, ts2)
	}
}

func TestInheritableBool_MarshalUnmarshalText(t *testing.T) {
	var b InheritableBool
	b.Set("TRUE")
	data, err := b.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	var b2 InheritableBool
	if err := b2.UnmarshalText(data); err != nil {
		t.Fatalf("UnmarshalText error: %v", err)
	}
	if b != b2 {
		t.Errorf("Marshal/Unmarshal mismatch: %v vs %v", b, b2)
	}
}

func TestInheritableString_MarshalUnmarshalText(t *testing.T) {
	var s InheritableString
	s.Set("foo")
	data, err := s.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	var s2 InheritableString
	if err := s2.UnmarshalText(data); err != nil {
		t.Fatalf("UnmarshalText error: %v", err)
	}
	if s != s2 {
		t.Errorf("Marshal/Unmarshal mismatch: %v vs %v", s, s2)
	}
}

func TestInheritableInt_MarshalUnmarshalText(t *testing.T) {
	var i InheritableInt
	i.Set("123")
	data, err := i.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	var i2 InheritableInt
	if err := i2.UnmarshalText(data); err != nil {
		t.Fatalf("UnmarshalText error: %v", err)
	}
	if i != i2 {
		t.Errorf("Marshal/Unmarshal mismatch: %v vs %v", i, i2)
	}
}

func TestInheritableBigInt_MarshalUnmarshalText(t *testing.T) {
	var i InheritableBigInt
	i.Set("123456789")
	data, err := i.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	var i2 InheritableBigInt
	if err := i2.UnmarshalText(data); err != nil {
		t.Fatalf("UnmarshalText error: %v", err)
	}
	if i != i2 {
		t.Errorf("Marshal/Unmarshal mismatch: %v vs %v", i, i2)
	}
}

func TestInheritableByte_MarshalUnmarshalText(t *testing.T) {
	var b InheritableByte
	b.Set("7")
	data, err := b.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	var b2 InheritableByte
	if err := b2.UnmarshalText(data); err != nil {
		t.Fatalf("UnmarshalText error: %v", err)
	}
	if b != b2 {
		t.Errorf("Marshal/Unmarshal mismatch: %v vs %v", b, b2)
	}
}

func TestSizeInBytes_Add(t *testing.T) {
	var s SizeInBytes
	s.Assign(100)
	s.Add(50)
	if s.Get() != 150 {
		t.Errorf("Add failed: got %d", s.Get())
	}
}

func TestTimespan_Add(t *testing.T) {
	var ts Timespan
	ts.Assign(100)
	ts.Add(50)
	if ts.Get() != 150 {
		t.Errorf("Add failed: got %d", ts.Get())
	}
}

func TestTimespan_Duration(t *testing.T) {
	var ts Timespan
	ts.Assign(1000)
	d := ts.Duration()
	if d != time.Microsecond*1000 {
		t.Errorf("Duration failed: got %v", d)
	}
}
