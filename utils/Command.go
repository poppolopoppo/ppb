package utils

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
)

var Commands = SharedMapT[string, func() *commandItem]{}
var ParsableFlags = SharedMapT[string, CommandParsableFlags]{}
var GlobalParsableFlags = commandItem{}

var LogCommand = NewLogCategory("Command")

/***************************************
 * CommandName
 ***************************************/

type CommandName struct {
	StringVar
}

func (x CommandName) AutoComplete(in AutoComplete) {
	for _, ci := range Commands.Values() {
		in.Add(ci().Details().Name)
	}
}

/***************************************
 * CommandLine
 ***************************************/

type CommandLine interface {
	PeekArg(int) (string, bool)
	ConsumeArg(int) (string, error)
	PersistentData
}

type CommandLinable interface {
	CommandLine(name, input string) (bool, error)
}

func splitArgsIFN(args []string, each func([]string) error) error {
	first := 0
	for last := 0; last < len(args); last++ {
		if strings.TrimSpace(args[last]) == `-and` {
			if first < last {
				if err := each(args[first:last]); err != nil {
					return err
				}
			}
			first = last + 1
		}
	}

	if first < len(args) {
		return each(args[first:])
	}

	return nil
}

func NewCommandLine(persistent PersistentData, args []string) (result []CommandLine) {
	splitArgsIFN(args, func(split []string) error {
		LogTrace(LogCommand, "process arguments -> %v", MakeStringer(func() string {
			return strings.Join(Map(func(a string) string {
				return fmt.Sprintf("%q", a)
			}, split...), ", ")
		}))

		result = append(result, &commandLine{
			args:           split,
			PersistentData: persistent,
		})
		return nil
	})

	return
}

type commandLine struct {
	args []string
	PersistentData
}

func (x *commandLine) PeekArg(i int) (string, bool) {
	if i >= len(x.args) {
		return "", false
	}
	return x.args[i], true
}
func (x *commandLine) ConsumeArg(i int) (string, error) {
	if i >= len(x.args) {
		return "", fmt.Errorf("missing argument(s)")
	}
	consumed := x.args[i]
	x.args = append(x.args[:i], x.args[i+1:]...)
	return consumed, nil
}

/***************************************
 * CommandEvents
 ***************************************/

type CommandContext interface {
	CommandItem
}

type CommandEvents struct {
	OnPrepare AnyEvent
	OnRun     AnyEvent
	OnClean   AnyEvent
	OnPanic   PublicEvent[error]
}

type commandEventError struct {
	cmd   *commandItem
	phase string
	inner error
}

func (x commandEventError) Error() string {
	return fmt.Sprintf("%s command %q failed with:\n\t%v", x.phase, x.cmd.Name, x.inner)
}

func makeCommandEventErrorIFN(cmd *commandItem, phase string, inner error) error {
	if inner == nil {
		return nil
	}
	return commandEventError{
		cmd:   cmd,
		phase: phase,
		inner: inner,
	}
}

func (x *CommandEvents) Bound() bool {
	return (x.OnPrepare.Bound() || x.OnRun.Bound() || x.OnClean.Bound() || x.OnPanic.Bound())
}
func (x *CommandEvents) Run() (err error) {
	if err = x.OnPrepare.Invoke(); err != nil {
		return
	}
	if err = x.OnRun.Invoke(); err != nil {
		return
	}
	if err = x.OnClean.Invoke(); err != nil {
		return
	}
	return
}
func (x *CommandEvents) Parse(cl CommandLine) (err error) {
	var name string
	if name, err = cl.ConsumeArg(0); err != nil {
		return
	}

	var cmd CommandItem
	if cmd, err = FindCommand(name); err == nil {
		AssertNotIn(cmd, nil)

		if err = cmd.Parse(cl); err == nil {
			x.Add(cmd.(*commandItem))
		}
	}

	return
}
func (x *CommandEvents) Add(it *commandItem) {
	if it.prepare != nil {
		x.OnPrepare.Add(AnyDelegate(func() error {
			return makeCommandEventErrorIFN(it, "prepare", it.prepare(it))
		}))
	}
	if it.run != nil {
		x.OnRun.Add(AnyDelegate(func() error {
			return makeCommandEventErrorIFN(it, "run", it.run(it))
		}))
	}
	if it.clean != nil {
		x.OnClean.Add(AnyDelegate(func() error {
			return makeCommandEventErrorIFN(it, "clean", it.clean(it))
		}))
	}
	if it.panic != nil {
		x.OnPanic.Add(it.panic)
	}
}

/***************************************
 * CommandArgument
 ***************************************/

type CommandArgumentFlag int32

const (
	COMMANDARG_PERSISTENT CommandArgumentFlag = iota
	COMMANDARG_CONSUME
	COMMANDARG_OPTIONAL
	COMMANDARG_VARIADIC
)

func (x CommandArgumentFlag) Ord() int32         { return int32(x) }
func (x *CommandArgumentFlag) FromOrd(ord int32) { *x = CommandArgumentFlag(ord) }
func (x CommandArgumentFlag) String() string {
	switch x {
	case COMMANDARG_PERSISTENT:
		return "PERSISTENT"
	case COMMANDARG_CONSUME:
		return "CONSUME"
	case COMMANDARG_OPTIONAL:
		return "OPTIONAL"
	case COMMANDARG_VARIADIC:
		return "VARIADIC"
	default:
		UnexpectedValue(x)
		return ""
	}
}
func (x *CommandArgumentFlag) Set(in string) error {
	switch strings.ToUpper(in) {
	case COMMANDARG_PERSISTENT.String():
		*x = COMMANDARG_PERSISTENT
	case COMMANDARG_CONSUME.String():
		*x = COMMANDARG_CONSUME
	case COMMANDARG_OPTIONAL.String():
		*x = COMMANDARG_OPTIONAL
	case COMMANDARG_VARIADIC.String():
		*x = COMMANDARG_VARIADIC
	default:
		return MakeUnexpectedValueError(x, x)
	}
	return nil
}

type CommandArgumentFlags = EnumSet[CommandArgumentFlag, *CommandArgumentFlag]

type CommandArgument interface {
	HasFlag(CommandArgumentFlag) bool
	Parse(CommandLine) error
	Format() string
	Help(*StructuredFile)
	AutoCompletable
}

/***************************************
 * commandBasicArgument
 ***************************************/

type commandBasicArgument struct {
	Short, Long string
	Description string
	Flags       CommandArgumentFlags
}

func (x *commandBasicArgument) HasFlag(flag CommandArgumentFlag) bool {
	return x.Flags.Has(flag)
}
func (x *commandBasicArgument) AutoComplete(in AutoComplete) {
	if len(x.Short) > 0 {
		in.Add(x.Short)
	}
	if len(x.Long) > 0 {
		in.Add(x.Long)
	}
}
func (x *commandBasicArgument) Parse(CommandLine) error {
	return nil
}

func (x *commandBasicArgument) Format() string {
	format := x.Short
	if len(x.Short) == 0 {
		Assert(func() bool { return len(x.Long) > 0 })
		format = x.Long
	} else if len(x.Long) > 0 {
		format = fmt.Sprint(format, "|", x.Long)
	}

	if x.Flags.Has(COMMANDARG_OPTIONAL) {
		format = fmt.Sprint(ANSI_FAINT, "[", format, "]")
		if x.Flags.Has(COMMANDARG_VARIADIC) {
			format = fmt.Sprint(format, "*")
		}
	} else {
		format = fmt.Sprint("<", format, ">")
		if x.Flags.Has(COMMANDARG_VARIADIC) {
			format = fmt.Sprint(format, "+")
		}
	}
	format = fmt.Sprint(ANSI_ITALIC, ANSI_FG0_YELLOW, format, ANSI_RESET)
	return format
}
func (x *commandBasicArgument) Help(w *StructuredFile) {
	w.Print("%s", x.Format())

	if enableInteractiveShell {
		w.Align(60)
		w.Println("%v%v%s%v", ANSI_FG1_BLACK, ANSI_FAINT, x.Flags, ANSI_RESET)
	}

	w.ScopeIndent(func() {
		w.Println("%v%s%v", ANSI_FG0_BLUE, x.Description, ANSI_RESET)
	})
}

/***************************************
 * CommandConsumeArgument
 ***************************************/

type commandConsumeOneArgument[T any, P interface {
	*T
	PersistentVar
}] struct {
	Value   *T
	Default T
	commandBasicArgument
}

func (x *commandConsumeOneArgument[T, P]) AutoComplete(in AutoComplete) {
	in.Any(x.Value)
}
func (x *commandConsumeOneArgument[T, P]) Parse(cl CommandLine) error {
	Assert(func() bool { return !(x.HasFlag(COMMANDARG_PERSISTENT) || x.HasFlag(COMMANDARG_VARIADIC)) })

	*x.Value = x.Default

	arg, err := cl.ConsumeArg(0)
	if err != nil {
		if x.HasFlag(COMMANDARG_OPTIONAL) {
			err = nil
		}
		return err
	}

	return P(x.Value).Set(arg)
}

func OptionCommandConsumeArg[T any, P interface {
	*T
	PersistentVar
}](name, description string, value *T, flags ...CommandArgumentFlag) CommandOptionFunc {
	return OptionCommandArg(&commandConsumeOneArgument[T, P]{
		Value:   value,
		Default: *value,
		commandBasicArgument: commandBasicArgument{
			Long:        name,
			Description: description,
			Flags:       MakeEnumSet(append(flags, COMMANDARG_CONSUME)...),
		},
	})
}

type commandConsumeManyArguments[T any, P interface {
	*T
	PersistentVar
}] struct {
	Value   *[]T
	Default []T
	commandBasicArgument
}

func (x *commandConsumeManyArguments[T, P]) AutoComplete(in AutoComplete) {
	var defaultScalar T
	in.Any(P(&defaultScalar))
}
func (x *commandConsumeManyArguments[T, P]) Parse(cl CommandLine) (err error) {
	Assert(func() bool { return !x.HasFlag(COMMANDARG_PERSISTENT) })

	*x.Value = []T{}

	var arg string
	for loop := 0; ; loop++ {
		if arg, err = cl.ConsumeArg(0); err == nil {
			var it T
			if err = P(&it).Set(arg); err == nil {
				*x.Value = append(*x.Value, it)
				continue
			}
		}

		if x.HasFlag(COMMANDARG_OPTIONAL) || loop > 0 {
			err = nil
		}
		break
	}

	return err
}

func OptionCommandConsumeMany[T any, P interface {
	*T
	PersistentVar
}](name, description string, value *[]T, flags ...CommandArgumentFlag) CommandOptionFunc {
	defaultValue := make([]T, len(*value))
	copy(defaultValue, *value)

	return OptionCommandArg(&commandConsumeManyArguments[T, P]{
		Value:   value,
		Default: defaultValue,
		commandBasicArgument: commandBasicArgument{
			Long:        name,
			Description: description,
			Flags:       MakeEnumSet(append(flags, COMMANDARG_CONSUME, COMMANDARG_VARIADIC)...),
		},
	})
}

/***************************************
 * CommandParsableFlagsArgument
 ***************************************/

type CommandFlagsVisitor interface {
	Persistent(name, usage string, value PersistentVar)
	Variable(name, usage string, value PersistentVar)
}

type CommandParsableFlags interface {
	Flags(CommandFlagsVisitor)
}

type commandPersistentVar struct {
	Name, Usage string
	Switch      string
	Value       PersistentVar
	Flags       CommandArgumentFlags
}

type commandParsableArgument struct {
	Value     CommandParsableFlags
	Variables []commandPersistentVar
	commandBasicArgument
}

func getCommandParsableFlagsName(value CommandParsableFlags) string {
	rt := reflect.TypeOf(value)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	return rt.Name()
}

func NewCommandParsableFlags[T any, P interface {
	*T
	CommandParsableFlags
}](flags *T) func() P {
	parsable := P(flags)
	ParsableFlags.Add(getCommandParsableFlagsName(parsable), parsable)
	RegisterSerializable(&CommandParsableBuilder[T, P]{})
	return func() P {
		return parsable
	}
}

func NewGlobalCommandParsableFlags[T any, P interface {
	*T
	CommandParsableFlags
}](description string, flags *T) func() P {
	parsable := P(flags)
	GlobalParsableFlags.Options(
		OptionCommandParsableFlags(
			getCommandParsableFlagsName(parsable),
			description,
			parsable))
	return NewCommandParsableFlags[T, P](flags)
}

func (x *commandParsableArgument) AutoComplete(in AutoComplete) {
	for _, v := range x.Variables {
		prefixed := NewPrefixedAutoComplete(v.Switch+"=", in)
		prefixed.Any(v.Value)
	}
}
func (x *commandParsableArgument) Parse(cl CommandLine) (err error) {
	for _, v := range x.Variables {
		if v.Flags.Has(COMMANDARG_PERSISTENT) {
			cl.LoadData(x.Long, v.Name, v.Value)
		}
	}

	for _, v := range x.Variables {
		for i := 0; ; {
			if arg, ok := cl.PeekArg(i); ok {
				var anon interface{} = v.Value
				var clb CommandLinable
				if clb, ok = anon.(CommandLinable); ok {
					ok, err = clb.CommandLine(v.Name, arg)
				} else {
					ok, err = InheritableCommandLine(v.Name, arg, v.Value)
				}

				if ok || err != nil {
					cl.ConsumeArg(i)
					if err == nil {
						continue
					} else {
						break
					}
				}
			} else {
				break
			}
			i++
		}

		if err != nil {
			break
		}
	}

	for _, v := range x.Variables {
		if v.Flags.Has(COMMANDARG_PERSISTENT) {
			cl.StoreData(x.Long, v.Name, v.Value)
		}
	}

	return
}
func (x *commandParsableArgument) Help(w *StructuredFile) {
	x.commandBasicArgument.Help(w)
	w.Println("")
	w.ScopeIndent(func() {
		for _, v := range x.Variables {
			colorFG, colorBG := ANSI_FG0_CYAN, ANSI_BG0_CYAN
			if v.Flags.Has(COMMANDARG_PERSISTENT) {
				colorFG, colorBG = ANSI_FG0_MAGENTA, ANSI_BG0_RED
			}

			printCommandBullet(w, colorBG)
			w.Print("%v%v-%s%v", ANSI_ITALIC, colorFG, v.Name, ANSI_RESET)

			w.Align(60)
			if v.Flags.Has(COMMANDARG_PERSISTENT) {
				CommandEnv.persistent.LoadData(x.Long, v.Name, v.Value)
			} else {
				w.Print("%v", ANSI_FAINT)
			}

			switch v.Value.(type) {
			case *StringVar, *Filename, *Directory:
				colorFG = ANSI_FG0_YELLOW
			case *IntVar, *BigIntVar:
				colorFG = ANSI_FG0_CYAN
			case *BoolVar:
				colorFG = ANSI_FG0_GREEN
			default:
				colorFG = ANSI_FG0_BLUE
			}

			w.Println("%v%v%s%v", ANSI_FRAME, colorFG, v.Value, ANSI_RESET)

			w.ScopeIndent(func() {
				w.Print("%v%v%s%v", ANSI_FG0_WHITE, ANSI_FAINT, v.Usage, ANSI_RESET)
			})
		}
	})
}

func newCommandParsableFlags(name, description string, value CommandParsableFlags, flags ...CommandArgumentFlag) *commandParsableArgument {
	arg := &commandParsableArgument{
		Value: value,
		commandBasicArgument: commandBasicArgument{
			Long:        getCommandParsableFlagsName(value),
			Description: description,
			Flags:       MakeEnumSet(append(flags, COMMANDARG_OPTIONAL, COMMANDARG_VARIADIC)...),
		},
	}

	VisitParsableFlags(arg.Value, func(name, usage string, value PersistentVar, persistent bool) {
		Assert(func() bool { return len(name) > 0 })
		Assert(func() bool { return len(usage) > 0 })

		v := commandPersistentVar{
			Name:   name,
			Usage:  usage,
			Switch: fmt.Sprint("-", name),
			Value:  value,
			Flags:  MakeEnumSet(COMMANDARG_OPTIONAL),
		}

		if persistent {
			v.Flags.Add(COMMANDARG_PERSISTENT)
		}

		arg.Variables = append(arg.Variables, v)
	})

	return arg
}

func OptionCommandParsableFlags(name, description string, value CommandParsableFlags, flags ...CommandArgumentFlag) CommandOptionFunc {
	arg := newCommandParsableFlags(name, description, value, flags...)
	return OptionCommandArg(arg)
}

func OptionCommandParsableAccessor[T CommandParsableFlags](name, description string, service func() T, flags ...CommandArgumentFlag) CommandOptionFunc {
	return func(ci *commandItem) {
		value := service()
		arg := newCommandParsableFlags(name, description, value, flags...)
		ci.arguments = append(ci.arguments, arg)
	}
}

/***************************************
 * CommandParsableBuilder
 ***************************************/

type CommandParsableBuilder[T any, P interface {
	*T
	CommandParsableFlags
}] struct {
	Name  string
	Flags T
}

func (x CommandParsableBuilder[T, P]) Alias() BuildAlias {
	return MakeBuildAlias("Command", "Flags", x.Name)
}
func (x *CommandParsableBuilder[T, P]) Build(BuildContext) error {
	if flags, ok := ParsableFlags.Get(x.Name); ok {
		x.Flags = *flags.(P)
	} else {
		return fmt.Errorf("could not find command parsable flags %q", x.Name)
	}

	VisitParsableFlags(P(&x.Flags), func(name, usage string, value PersistentVar, persistent bool) {
		if persistent {
			CommandEnv.persistent.LoadData(x.Name, name, value)
		}
	})
	return nil
}
func (x *CommandParsableBuilder[T, P]) Serialize(ar Archive) {
	ar.String(&x.Name)
	SerializeParsableFlags(ar, P(&x.Flags))
}

type commandParsableFunctor struct {
	onPersistent func(name, usage string, value PersistentVar, persistent bool)
}

func (x commandParsableFunctor) Persistent(name, usage string, value PersistentVar) {
	x.onPersistent(name, usage, value, true)
}
func (x commandParsableFunctor) Variable(name, usage string, value PersistentVar) {
	x.onPersistent(name, usage, value, false)
}

func VisitParsableFlags(parsable CommandParsableFlags,
	onPersistent func(name, usage string, value PersistentVar, persistent bool)) {
	parsable.Flags(commandParsableFunctor{onPersistent: onPersistent})
}

func SerializeParsableFlags(ar Archive, parsable CommandParsableFlags) {
	VisitParsableFlags(parsable, func(name, usage string, value PersistentVar, persistent bool) {
		if persistent {
			ar.Serializable(value)
		}
	})
}

func GetBuildableFlags[T any, P interface {
	*T
	CommandParsableFlags
}](flags *T) BuildFactoryTyped[*CommandParsableBuilder[T, P]] {
	return MakeBuildFactory(func(bi BuildInitializer) (CommandParsableBuilder[T, P], error) {
		return CommandParsableBuilder[T, P]{
			Name:  getCommandParsableFlagsName(P(flags)),
			Flags: *flags,
		}, nil
	})
}

/***************************************
 * CommandItem
 ***************************************/

type CommandDetails struct {
	Category, Name string
	Description    string
	Notes          string
}

func (x CommandDetails) IsNaked() bool {
	return len(x.Name) == 0
}

type CommandItem interface {
	Details() CommandDetails
	Arguments() []CommandArgument
	Options(...CommandOptionFunc)
	Parse(CommandLine) error
	Usage() string
	Help(*StructuredFile)
	AutoCompletable
	fmt.Stringer
}

type commandItem struct {
	CommandDetails

	arguments []CommandArgument

	prepare EventDelegate[CommandContext]
	run     EventDelegate[CommandContext]
	clean   EventDelegate[CommandContext]
	panic   EventDelegate[error]
}

func (x *commandItem) Details() CommandDetails      { return x.CommandDetails }
func (x *commandItem) Arguments() []CommandArgument { return x.arguments }
func (x *commandItem) String() string               { return fmt.Sprint(x.Category, "/", x.Name) }

func (x *commandItem) Options(options ...CommandOptionFunc) {
	for _, opt := range options {
		opt(x)
	}
}
func (x *commandItem) AutoComplete(in AutoComplete) {
	for _, a := range x.Arguments() {
		a.AutoComplete(in)
	}
}
func (x *commandItem) Parse(cl CommandLine) error {
	// first switch/non-positional arguments
	for _, it := range x.arguments {
		if it.HasFlag(COMMANDARG_CONSUME) {
			continue
		}
		if err := it.Parse(cl); err != nil {
			return err
		}
	}

	// then detect unknown command flags (warning)
	if !x.IsNaked() {
		unknownFlags := []string{}
		for i := 0; ; {
			if arg, ok := cl.PeekArg(i); ok {
				if len(arg) > 0 && arg[0] == '-' {
					cl.ConsumeArg(i) // consume the arg: it will ignored consumable/positional arguments
					if arg == "--" {
						// special case: using "--" will ignore option parsing for the rest of the command-line
						break
					}
					unknownFlags = append(unknownFlags, arg)
					continue
				}
				i++
			} else {
				break
			}
		}
		if len(unknownFlags) > 0 {
			// report a warning about unknown flag: dont' die on thie
			LogWarning(LogCommand, "unknown command flags: %q", strings.Join(unknownFlags, ", "))
		}
	}

	// then consume arguments (error)
	for _, it := range x.arguments {
		if !it.HasFlag(COMMANDARG_CONSUME) {
			continue
		}
		if err := it.Parse(cl); err != nil {
			return err
		}
	}

	// then detect unused arguments
	if !x.IsNaked() {
		unusedArguments := []string{}
		for i := 0; ; i++ {
			if arg, ok := cl.PeekArg(i); ok {
				unusedArguments = append(unusedArguments, arg)
			} else {
				break
			}
		}
		if len(unusedArguments) > 0 {
			// unused arguments will fail command parsing (avoid messing up on user mistake)
			return fmt.Errorf("unused command arguments: %q", strings.Join(unusedArguments, ", "))
		}
	}

	return nil
}
func (x *commandItem) Usage() (format string) {
	if enableInteractiveShell {
		format = fmt.Sprint(
			ANSI_BG0_MAGENTA, "*", ANSI_RESET, " ",
			ANSI_UNDERLINE, ANSI_OVERLINE, ANSI_FG1_GREEN, x.Name, ANSI_RESET)
	} else {
		format = x.Name
	}

	for _, a := range x.arguments {
		format = fmt.Sprint(format, " ", a.Format())
	}
	return format
}
func (x *commandItem) Help(w *StructuredFile) {
	if w.Minify() {
		w.Println(" %s%-20s%s %s", ANSI_FG1_GREEN, x.Name, ANSI_RESET, x.Description)
	} else {
		w.Println("%s", x.Usage())
		w.ScopeIndent(func() {
			w.Println("%s", x.Description)
			w.Println("")

			if len(x.Notes) > 0 {
				w.Println(x.Notes)
				w.Println("")
			}

			w.ScopeIndent(func() {
				for _, a := range x.arguments {
					printCommandBullet(w, ANSI_BG0_YELLOW)
					a.Help(w)
					w.LineBreak()
					w.Println("")
				}
			})
		})
	}
}

func printCommandBullet(w *StructuredFile, color AnsiCode) {
	// w.Print("%v*%v ", color, ANSI_RESET)
}

/***************************************
 * NewCommand
 ***************************************/

type CommandOptionFunc func(*commandItem)

func OptionCommandArg(arg CommandArgument) CommandOptionFunc {
	return func(ci *commandItem) {
		ci.arguments = append(ci.arguments, arg)
	}
}
func OptionCommandPrepare(e EventDelegate[CommandContext]) CommandOptionFunc {
	return func(ci *commandItem) {
		ci.prepare = func(cc CommandContext) error {
			LogTrace(LogCommand, "prepare %q command", ci)
			return e(cc)
		}
	}
}
func OptionCommandRun(e EventDelegate[CommandContext]) CommandOptionFunc {
	return func(ci *commandItem) {
		ci.run = func(cc CommandContext) error {
			LogTrace(LogCommand, "run %q command", ci)
			return e(cc)
		}
	}
}
func OptionCommandClean(e EventDelegate[CommandContext]) CommandOptionFunc {
	return func(ci *commandItem) {
		ci.clean = func(cc CommandContext) error {
			LogTrace(LogCommand, "clean %q command", ci)
			return e(cc)
		}
	}
}
func OptionCommandPanic(e EventDelegate[error]) CommandOptionFunc {
	return func(ci *commandItem) {
		ci.panic = func(err error) error {
			LogTrace(LogCommand, "panic %q command", ci)
			return e(err)
		}
	}
}
func OptionCommandNotes(format string, args ...interface{}) CommandOptionFunc {
	return func(ci *commandItem) {
		ci.Notes += fmt.Sprintf(format, args...)
	}
}
func OptionCommandItem(fn func(CommandItem)) CommandOptionFunc {
	return func(ci *commandItem) {
		fn(ci)
	}
}

func NewCommand(
	category, name, description string,
	options ...CommandOptionFunc,
) func() CommandItem {
	result := Memoize(func() *commandItem {
		result := &commandItem{
			CommandDetails: CommandDetails{
				Category:    category,
				Name:        name,
				Description: description,
			},
		}
		result.Options(options...)
		return result
	})
	key := strings.ToUpper(name)
	Commands.FindOrAdd(key, result)
	return func() CommandItem {
		if factory, ok := Commands.Get(key); ok {
			return factory()
		} else {
			LogPanic(LogCommand, "command %q not found", name)
			return nil
		}
	}
}

/***************************************
 * Commandable
 ***************************************/

type Commandable interface {
	Init(CommandContext) error
	Run(CommandContext) error
}

type commandablePrepare interface {
	Prepare(CommandContext) error
}
type commandableClean interface {
	Clean(CommandContext) error
}
type commandablePanic interface {
	Panic(error) error
}

func NewCommandable[T any, P interface {
	*T
	Commandable
}](category, name, description string, cmd *T) func() CommandItem {
	var options []CommandOptionFunc

	options = append(options, func(ci *commandItem) { P(cmd).Init(ci) })
	options = append(options, OptionCommandRun(P(cmd).Run))

	var anon interface{} = cmd
	if it, ok := anon.(commandablePrepare); ok {
		options = append(options, OptionCommandPrepare(it.Prepare))
	}
	if it, ok := anon.(commandableClean); ok {
		options = append(options, OptionCommandClean(it.Clean))
	}
	if it, ok := anon.(commandablePanic); ok {
		options = append(options, OptionCommandPanic(it.Panic))
	}

	return NewCommand(category, name, description, options...)
}

/***************************************
 * AllCommands
 ***************************************/

func GetAllCommands() []CommandItem {
	cmds := Map(func(it func() *commandItem) *commandItem {
		return it()
	}, Commands.Values()...)
	sort.Slice(cmds, func(i, j int) bool {
		if c := strings.Compare(cmds[i].Category, cmds[j].Category); c == 0 {
			return strings.Compare(cmds[i].Name, cmds[j].Name) < 0
		} else {
			return c < 0
		}
	})
	return Map(func(it *commandItem) CommandItem { return it }, cmds...)
}

func FindCommand(name string) (CommandItem, error) {
	if cmd, found := Commands.Get(strings.ToUpper(name)); found {
		return cmd(), nil
	} else {
		return nil, fmt.Errorf("unknown command %q", name)
	}
}

func PrintCommandHelp(w io.Writer, detailed bool) {
	restoreLogLevel := gLogger.SetLevelMaximum(LOG_VERBOSE)
	defer gLogger.SetLevel(restoreLogLevel)

	f := NewStructuredFile(w, "  ", !detailed)

	f.Print(`
%v  v.%v  [%v]
build-system for PoPpOlOpPoPo Engine

  %vcompiled on %v%v`,
		PROCESS_INFO.Path, PROCESS_INFO.Version, GetProcessSeed().ShortString(),
		ANSI_FG1_BLACK, PROCESS_INFO.Timestamp.Local(), ANSI_RESET)

	header := func(title string) {
		f.Print("%v%v", ANSI_FG1_MAGENTA, ANSI_FAINT)
		f.Pad(59, "-")
		f.Print(" %s ", title)
		f.Pad(80, "-")
		f.Println("%v", ANSI_RESET)
	}

	f.Println("")
	f.BeginIndent()
	lastCategory := ""

	for _, cmd := range GetAllCommands() {
		details := cmd.Details()
		if lastCategory != details.Category {
			lastCategory = details.Category
			f.EndIndent()
			if !detailed {
				f.Println("")
			}

			header(details.Category)

			f.BeginIndent()
			if detailed {
				f.Println("")
			}
		}

		cmd.Help(f)
	}
	f.EndIndent()

	if len(GlobalParsableFlags.arguments) > 0 {
		if !detailed {
			f.Println("")
		}

		header("Global")
		f.ScopeIndent(func() {
			f.Println("")
			for _, a := range GlobalParsableFlags.arguments {
				a.Help(f)
			}
		})

		f.Println("")
	}
}

/***************************************
 * RunCommands
 ***************************************/

func PrepareCommands(cls []CommandLine) error {
	for _, cl := range cls {
		if err := GlobalParsableFlags.Parse(cl); err != nil {
			return err
		}
	}
	return nil
}

func ParseCommand(cl CommandLine, events *CommandEvents) (err error) {
	var name string
	if name, err = cl.ConsumeArg(0); err != nil {
		return
	}

	var cmd CommandItem
	if cmd, err = FindCommand(name); err == nil {
		AssertNotIn(cmd, nil)

		if err = cmd.Parse(cl); err == nil {
			events.Add(cmd.(*commandItem))
		}
	}

	return
}

/***************************************
 * HelpCommand
 ***************************************/

type HelpCommand struct {
	Command CommandName
}

func (x *HelpCommand) Init(cc CommandContext) error {
	cc.Options(OptionCommandConsumeArg("command_name", "print specific informations if a command name is provided", &x.Command, COMMANDARG_OPTIONAL))
	return nil
}
func (x *HelpCommand) Run(cc CommandContext) (err error) {
	var cmd CommandItem
	if !x.Command.IsInheritable() {
		cmd, err = FindCommand(x.Command.Get())
	}

	var w io.Writer = os.Stdout
	if cmd == nil {
		PrintCommandHelp(w, IsLogLevelActive(LOG_VERBOSE))
	} else {
		f := NewStructuredFile(w, "  ", false)

		f.Println("")
		f.ScopeIndent(func() {
			cmd.Help(f)
		})
	}

	return nil
}

var CommandHelp = NewCommandable("Misc", "help", "print help about command usage", &HelpCommand{})

/***************************************
 * AutoComplete
 ***************************************/

type AutoCompleteCommand struct {
	Command CommandName
	Input   StringVar
}

func (x *AutoCompleteCommand) Init(cc CommandContext) error {
	cc.Options(
		OptionCommandConsumeArg("command_name", "selected command for auto-completion", &x.Command),
		OptionCommandConsumeArg("input_text", "text query for command-line auto-completion", &x.Input),
	)
	return nil
}
func (x *AutoCompleteCommand) Run(cc CommandContext) error {
	cmd, err := FindCommand(x.Command.Get())
	if err != nil {
		return err
	}

	autocomplete := NewAutoComplete(x.Input.Get())
	autocomplete.Append(&GlobalParsableFlags)
	autocomplete.Append(cmd)

	for _, o := range autocomplete.Results(20) {
		LogForward(o)
	}
	return nil
}

var CommandAutoComplete = NewCommandable("Misc", "autocomplete", "run auto-completion", &AutoCompleteCommand{})
