package utils

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
)

var LogCommand = base.NewLogCategory("Command")

var AllCommands = base.SharedMapT[string, CommandDescriptor]{}

var GlobalParsableFlags commandItem

/***************************************
 * CommandDescriptor
 ***************************************/

type CommandDescriptor struct {
	Create func() CommandItem
	CommandDetails
}

/***************************************
 * CommandName
 ***************************************/

type CommandName struct {
	StringVar
}

func (x CommandName) Compare(o CommandName) int {
	return x.StringVar.Compare(o.StringVar)
}
func (x CommandName) AutoComplete(in base.AutoComplete) {
	for _, ci := range AllCommands.Values() {
		in.Add(ci.Name, ci.Description)
	}
}

/***************************************
 * CommandLine
 ***************************************/

type CommandLine interface {
	Empty() bool
	PeekArg(int) (string, bool)
	ConsumeArg(int) (string, bool)
	fmt.Stringer
	PersistentData
}

type CommandLinable interface {
	CommandLine(name, input string) (bool, error)
}

func splitArgsIFN(args []string, each func([]string) error) error {
	first := 0
	for last := 0; last < len(args); last++ {
		if strings.TrimSpace(args[last]) == `--` {
			break // '--' disables all command-line switches
		}
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
		base.LogTrace(LogCommand, "process arguments -> %v", base.MakeStringer(func() string {
			return strings.Join(base.Map(func(a string) string {
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

func (x *commandLine) Empty() bool {
	return len(x.args) == 0
}
func (x *commandLine) String() string {
	return strings.Join(x.args, " ")
}
func (x *commandLine) PeekArg(i int) (string, bool) {
	if i >= len(x.args) {
		return "", false
	}
	return x.args[i], true
}
func (x *commandLine) ConsumeArg(i int) (string, bool) {
	if i >= len(x.args) {
		return "", false
	}
	consumed := x.args[i]
	x.args = base.Delete(x.args, i)
	return consumed, true
}

/***************************************
 * CommandEvents
 ***************************************/

type CommandContext interface {
	CommandItem
}

type CommandEvents struct {
	OnPrepare base.AnyEvent
	OnRun     base.AnyEvent
	OnClean   base.AnyEvent
	OnPanic   base.PublicEvent[error]
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
	name, ok := cl.ConsumeArg(0)
	if !ok {
		return fmt.Errorf("missing command name, use `help` to learn about command usage")
	}

	var cmd CommandItem
	if cmd, err = FindCommand(name); err == nil {
		base.AssertNotIn(cmd, nil)

		if err = cmd.Parse(cl); err == nil {
			x.Add(cmd.(*commandItem))
		}
	}

	return
}
func (x *CommandEvents) Add(it *commandItem) {
	if it.prepare.Bound() {
		x.OnPrepare.Add(base.AnyDelegate(func() error {
			defer base.LogBenchmark(LogCommand, "prepare command %q", it).Close()
			return makeCommandEventErrorIFN(it, "prepare", it.prepare.Invoke(it))
		}))
	}
	if it.run.Bound() {
		x.OnRun.Add(base.AnyDelegate(func() error {
			defer base.LogBenchmark(LogCommand, "run command %q", it).Close()
			return makeCommandEventErrorIFN(it, "run", it.run.Invoke(it))
		}))
	}
	if it.clean.Bound() {
		x.OnClean.Add(base.AnyDelegate(func() error {
			defer base.LogBenchmark(LogCommand, "clean command %q", it).Close()
			return makeCommandEventErrorIFN(it, "clean", it.clean.Invoke(it))
		}))
	}
	if it.panic.Bound() {
		x.OnPanic.Add(func(err error) error {
			defer base.LogBenchmark(LogCommand, "panic command %q: %v", it, err).Close()
			return it.panic.Invoke(err)
		})
	}
}

/***************************************
 * CommandArgument
 ***************************************/

type CommandArgumentFlag byte

const (
	COMMANDARG_PERSISTENT CommandArgumentFlag = iota
	COMMANDARG_CONSUME
	COMMANDARG_OPTIONAL
	COMMANDARG_VARIADIC
)

func GetCommandArgumentFlags() []CommandArgumentFlag {
	return []CommandArgumentFlag{
		COMMANDARG_PERSISTENT,
		COMMANDARG_CONSUME,
		COMMANDARG_OPTIONAL,
		COMMANDARG_VARIADIC,
	}
}

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
		base.UnexpectedValue(x)
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
		return base.MakeUnexpectedValueError(x, x)
	}
	return nil
}
func (x CommandArgumentFlag) Description() string {
	switch x {
	case COMMANDARG_PERSISTENT:
		return "value is stored in the config and restored at every launch"
	case COMMANDARG_CONSUME:
		return "value does not expect a prefix switch, and should be consumed from command-line first free argument"
	case COMMANDARG_OPTIONAL:
		return "value does not need to specified on the command-line"
	case COMMANDARG_VARIADIC:
		return "multiple values can be specified"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x CommandArgumentFlag) AutoComplete(in base.AutoComplete) {
	for _, it := range GetCommandArgumentFlags() {
		in.Add(it.String(), it.Description())
	}
}

type CommandArgumentFlags = base.EnumSet[CommandArgumentFlag, *CommandArgumentFlag]

type CommandArgumentDetails struct {
	Short, Long string
	Description string
	Flags       CommandArgumentFlags
}

type CommandArgument interface {
	Details() CommandArgumentDetails
	Inspect(func(CommandArgumentDetails, PersistentVar) error) error
	HasFlag(CommandArgumentFlag) bool
	Parse(CommandLine) error
	Format() string
	Help(*base.StructuredFile)
	base.AutoCompletable
}

/***************************************
 * commandBasicArgument
 ***************************************/

type commandBasicArgument struct {
	CommandArgumentDetails
}

func (x *commandBasicArgument) Name() string {
	if len(x.Long) > 0 {
		return x.Long
	}
	return x.Short
}
func (x *commandBasicArgument) Details() CommandArgumentDetails {
	return x.CommandArgumentDetails
}
func (x *commandBasicArgument) HasFlag(flag CommandArgumentFlag) bool {
	return x.Flags.Has(flag)
}
func (x commandBasicArgument) AutoComplete(in base.AutoComplete) {
	if len(x.Short) > 0 {
		in.Add(x.Short, x.Description)
	}
	if len(x.Long) > 0 {
		in.Add(x.Long, x.Description)
	}
}
func (x *commandBasicArgument) Parse(CommandLine) error {
	return nil
}

func (x *commandBasicArgument) Format() string {
	format := x.Short
	if len(x.Short) == 0 {
		base.Assert(func() bool { return len(x.Long) > 0 })
		format = x.Long
	} else if len(x.Long) > 0 {
		format = fmt.Sprint(format, "|", x.Long)
	}

	if x.Flags.Has(COMMANDARG_OPTIONAL) {
		format = fmt.Sprint(base.ANSI_FAINT, "[", format, "]")
		if x.Flags.Has(COMMANDARG_VARIADIC) {
			format = fmt.Sprint(format, "*")
		}
	} else {
		format = fmt.Sprint("<", format, ">")
		if x.Flags.Has(COMMANDARG_VARIADIC) {
			format = fmt.Sprint(format, "+")
		}
	}
	format = fmt.Sprint(base.ANSI_ITALIC, base.ANSI_FG0_YELLOW, format, base.ANSI_RESET)
	return format
}
func (x *commandBasicArgument) Help(w *base.StructuredFile) {
	if w.Minify() {
		w.Print("%s", x.Format())
		w.Align(30)
		w.Println("%v%s%v", base.ANSI_FG0_BLUE, x.Description, base.ANSI_RESET)

	} else {
		w.Print("%s", x.Format())

		if base.EnableInteractiveShell() {
			w.Align(60)
			w.Println("%v%v%s%v", base.ANSI_FG1_BLACK, base.ANSI_FAINT, x.Flags, base.ANSI_RESET)
		}

		w.ScopeIndent(func() {
			w.Println("%v%s%v", base.ANSI_FG0_BLUE, x.Description, base.ANSI_RESET)
		})
	}
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

func (x *commandConsumeOneArgument[T, P]) Inspect(each func(CommandArgumentDetails, PersistentVar) error) error {
	return each(x.CommandArgumentDetails, P(x.Value))
}
func (x *commandConsumeOneArgument[T, P]) AutoComplete(in base.AutoComplete) {
	if err := in.Any(P(x.Value)); err == nil {
		base.LogTrace(base.LogAutoComplete, "consume one %q", x.Name())
	} else {
		base.LogWarningVerbose(base.LogAutoComplete, "consume one %q: %v", x.Name(), err)
	}
}
func (x *commandConsumeOneArgument[T, P]) Parse(cl CommandLine) error {
	base.Assert(func() bool { return !(x.HasFlag(COMMANDARG_PERSISTENT) || x.HasFlag(COMMANDARG_VARIADIC)) })

	*x.Value = x.Default

	if arg, ok := cl.ConsumeArg(0); ok || x.HasFlag(COMMANDARG_OPTIONAL) {
		return P(x.Value).Set(arg)
	}
	return fmt.Errorf("missing required argument for %q, check command usage with `help`", x.Name())
}

func OptionCommandConsumeArg[T any, P interface {
	*T
	PersistentVar
}](name, description string, value *T, flags ...CommandArgumentFlag) CommandOptionFunc {
	return OptionCommandArg(&commandConsumeOneArgument[T, P]{
		Value:   value,
		Default: *value,
		commandBasicArgument: commandBasicArgument{
			CommandArgumentDetails{
				Long:        name,
				Description: description,
				Flags:       base.NewEnumSet(append(flags, COMMANDARG_CONSUME)...),
			},
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

func (x *commandConsumeManyArguments[T, P]) Inspect(each func(CommandArgumentDetails, PersistentVar) error) error {
	for i := range *x.Value {
		if err := each(x.CommandArgumentDetails, P(&(*x.Value)[i])); err != nil {
			return err
		}
	}
	return nil
}
func (x *commandConsumeManyArguments[T, P]) Details() CommandArgumentDetails {
	return x.CommandArgumentDetails
}
func (x *commandConsumeManyArguments[T, P]) AutoComplete(in base.AutoComplete) {
	var defaultScalar T
	if err := in.Any(P(&defaultScalar)); err == nil {
		base.LogTrace(base.LogAutoComplete, "consume many %q", x.Name())
	} else {
		base.LogWarningVerbose(base.LogAutoComplete, "consume many %q: %v", x.Name(), err)
	}
}
func (x *commandConsumeManyArguments[T, P]) Parse(cl CommandLine) (err error) {
	base.Assert(func() bool { return !x.HasFlag(COMMANDARG_PERSISTENT) })

	*x.Value = []T{}

	for loop := 0; ; loop++ {
		if arg, ok := cl.ConsumeArg(0); ok {
			var it T
			if err = P(&it).Set(arg); err == nil {
				*x.Value = append(*x.Value, it)
				continue
			}
		} else if !x.HasFlag(COMMANDARG_OPTIONAL) && loop == 0 {
			return fmt.Errorf("missing at least one argument for %q list, check command usage with `help`", x.Name())
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
			CommandArgumentDetails{
				Long:        name,
				Description: description,
				Flags:       base.NewEnumSet(append(flags, COMMANDARG_CONSUME, COMMANDARG_VARIADIC)...),
			},
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

func NewCommandParsableFlags[T any, P interface {
	*T
	CommandParsableFlags
}](flags *T) func() P {
	parsable := P(flags)
	return func() P {
		return parsable
	}
}

func NewGlobalCommandParsableFlags[T any, P interface {
	*T
	CommandParsableFlags
}](description string, flags *T, options ...CommandOptionFunc) func() P {
	parsable := P(flags)
	options = append(options, OptionCommandParsableFlags(
		base.GetTypename(parsable),
		description,
		parsable))
	GlobalParsableFlags.Options(options...)
	return NewCommandParsableFlags[T, P](flags)
}

func (x *commandParsableArgument) Inspect(each func(CommandArgumentDetails, PersistentVar) error) error {
	for _, it := range x.Variables {
		if err := each(CommandArgumentDetails{
			Long:        it.Name,
			Short:       it.Switch,
			Description: it.Usage,
			Flags:       it.Flags,
		}, it.Value); err != nil {
			return err
		}
	}
	return nil
}

func (x commandParsableArgument) AutoComplete(in base.AutoComplete) {
	for _, v := range x.Variables {
		if boolean, ok := v.Value.(*BoolVar); ok {
			boolean.AutoCompleteFlag(in, v.Switch, v.Usage)
		} else {
			prefixed := base.NewPrefixedAutoComplete(
				v.Switch+"=",
				v.Usage,
				in)
			if err := prefixed.Any(v.Value); err == nil {
				base.LogTrace(base.LogAutoComplete, "parsable \"%s/%s\"", x.Name(), v.Name)
			} else {
				base.LogWarningVerbose(base.LogAutoComplete, "parsable \"%s/%s\": %v", x.Name(), v.Name, err)
				in.Add(fmt.Sprint(v.Switch, `=`, v.Value.String()), v.Usage)
			}
		}
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
				if arg == "--" {
					// special case: using "--" will ignore option parsing for the rest of the command-line
					break
				}

				var anon interface{} = v.Value
				var clb CommandLinable
				if clb, ok = anon.(CommandLinable); ok {
					ok, err = clb.CommandLine(v.Name, arg)
				} else {
					ok, err = base.InheritableCommandLine(v.Name, arg, v.Value)
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
func (x *commandParsableArgument) Help(w *base.StructuredFile) {
	x.commandBasicArgument.Help(w)
	w.Println("")
	w.ScopeIndent(func() {
		for _, v := range x.Variables {
			colorFG := base.ANSI_FG0_CYAN
			if v.Flags.Has(COMMANDARG_PERSISTENT) {
				colorFG = base.ANSI_FG1_MAGENTA
			}

			w.Print("%v%v-%s%v", base.ANSI_ITALIC, colorFG, v.Name, base.ANSI_RESET)

			if w.Minify() {
				w.Align(30)
				w.Println("%v%v%s%v", base.ANSI_FG0_WHITE, base.ANSI_FAINT, v.Usage, base.ANSI_RESET)
				continue
			}

			w.Align(60)
			if v.Flags.Has(COMMANDARG_PERSISTENT) {
				CommandEnv.persistent.LoadData(x.Long, v.Name, v.Value)
			} else {
				w.Print("%v", base.ANSI_FAINT)
			}

			switch v.Value.(type) {
			case *StringVar, *Filename, *Directory:
				colorFG = base.ANSI_FG1_YELLOW
			case *IntVar, *BigIntVar:
				colorFG = base.ANSI_FG1_CYAN
			case *BoolVar:
				colorFG = base.ANSI_FG1_GREEN
			default:
				colorFG = base.ANSI_FG1_BLUE
			}

			var allowedValues []base.AutoCompleteResult
			if readPort := CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "Help"}); readPort != nil {
				allowedValues = base.GatherAutoCompletionFrom(v.Value, readPort)
				readPort.Close()
			}

			if len(allowedValues) > 0 {
				sb := strings.Builder{}

				sb.WriteString(base.ANSI_FRAME.String())
				sb.WriteString(colorFG.String())
				sb.WriteString(v.Value.String())
				sb.WriteString(base.ANSI_RESET.String())

				sb.WriteString(colorFG.String())
				sb.WriteString(base.ANSI_FAINT.String())
				sb.WriteString(" \t(")
				for i, it := range allowedValues {
					if i > 0 {
						sb.WriteRune('|')
					}
					sb.WriteString(it.Text)
				}
				sb.WriteRune(')')
				sb.WriteString(base.ANSI_RESET.String())

				w.Println(sb.String())
			} else {
				w.Println("%v%v%s%v", base.ANSI_FRAME, colorFG, v.Value, base.ANSI_RESET)
			}

			w.ScopeIndent(func() {
				w.Print("%v%v%s%v", base.ANSI_FG0_WHITE, base.ANSI_FAINT, v.Usage, base.ANSI_RESET)
			})
		}
	})
}

func newCommandParsableFlags(name, description string, value CommandParsableFlags, flags ...CommandArgumentFlag) *commandParsableArgument {
	arg := &commandParsableArgument{
		Value: value,
		commandBasicArgument: commandBasicArgument{
			CommandArgumentDetails{
				Long:        name,
				Description: description,
				Flags:       base.NewEnumSet(append(flags, COMMANDARG_OPTIONAL, COMMANDARG_VARIADIC)...),
			},
		},
	}

	VisitParsableFlags(arg.Value, func(name, usage string, value PersistentVar, persistent bool) {
		base.Assert(func() bool { return len(name) > 0 })
		base.Assert(func() bool { return len(usage) > 0 })

		v := commandPersistentVar{
			Name:   name,
			Usage:  usage,
			Switch: fmt.Sprint("-", name),
			Value:  value,
			Flags:  base.NewEnumSet(COMMANDARG_OPTIONAL),
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

func OptionCommandParsableAccessor[T CommandParsableFlags](name, description string, getter func() T, flags ...CommandArgumentFlag) CommandOptionFunc {
	return func(ci *commandItem) {
		value := getter()
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
func (x *CommandParsableBuilder[T, P]) Build(bc BuildContext) error {
	bc.Annotate(AnnocateBuildMute) // don't display build flags output by default
	VisitParsableFlags(P(&x.Flags), func(name, usage string, value PersistentVar, persistent bool) {
		if persistent {
			CommandEnv.persistent.LoadData(x.Name, name, value)
		}
	})
	return nil
}
func (x *CommandParsableBuilder[T, P]) Serialize(ar base.Archive) {
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

func SerializeParsableFlags(ar base.Archive, parsable CommandParsableFlags) {
	VisitParsableFlags(parsable, func(name, usage string, value PersistentVar, persistent bool) {
		if persistent {
			ar.Serializable(value)
		}
	})
}

func NewCommandParsableFactory[T any, P interface {
	*T
	CommandParsableFlags
}](name string, flags T) BuildFactoryTyped[*CommandParsableBuilder[T, P]] {
	base.RegisterSerializable[CommandParsableBuilder[T, P]]()
	return MakeBuildFactory(func(bi BuildInitializer) (CommandParsableBuilder[T, P], error) {
		return CommandParsableBuilder[T, P]{
			Name:  name,
			Flags: flags,
		}, nil
	})
}

/***************************************
 * CommandDetails
 ***************************************/

type CommandDetails struct {
	Category, Name string
	Description    string
	Notes          string
}

func (x CommandDetails) GetCommandName() (result CommandName) {
	result.Assign(x.Name)
	return
}
func (x CommandDetails) IsNaked() bool {
	return len(x.Name) == 0
}
func (x CommandDetails) Compare(o CommandDetails) int {
	if c := strings.Compare(x.Category, o.Category); c != 0 {
		return c
	} else {
		return strings.Compare(x.Name, o.Name)
	}
}

/***************************************
 * CommandItem
 ***************************************/

type CommandItem interface {
	Details() CommandDetails
	Arguments() []CommandArgument
	Options(...CommandOptionFunc)
	Parse(CommandLine) error
	Usage() string
	Help(*base.StructuredFile)
	base.AutoCompletable
	fmt.Stringer
}

type commandItem struct {
	CommandDetails

	arguments []CommandArgument

	prepare base.PublicEvent[CommandContext]
	run     base.PublicEvent[CommandContext]
	clean   base.PublicEvent[CommandContext]
	panic   base.PublicEvent[error]
}

func (x *commandItem) Details() CommandDetails      { return x.CommandDetails }
func (x *commandItem) Arguments() []CommandArgument { return x.arguments }
func (x *commandItem) String() string               { return fmt.Sprint(x.Category, "/", x.Name) }

func (x *commandItem) Options(options ...CommandOptionFunc) {
	for _, opt := range options {
		opt(x)
	}
}
func (x commandItem) AutoComplete(in base.AutoComplete) {
	base.LogTrace(base.LogAutoComplete, "autocomplete command %q with %d arguments", x.Name, len(x.arguments))
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
					cl.ConsumeArg(i) // consume the arg: it will be ignored by consumable/positional arguments
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
			base.LogWarning(LogCommand, "unknown command flags: %q", strings.Join(unknownFlags, ", "))
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
	if base.EnableInteractiveShell() {
		format = fmt.Sprint(
			base.ANSI_BG0_MAGENTA, "*", base.ANSI_RESET, " ",
			base.ANSI_UNDERLINE, base.ANSI_OVERLINE, base.ANSI_FG1_GREEN, x.Name, base.ANSI_RESET)
	} else {
		format = x.Name
	}

	for _, a := range x.arguments {
		format = fmt.Sprint(format, " ", a.Format())
	}
	return format
}
func (x *commandItem) Help(w *base.StructuredFile) {
	if w.Minify() {
		w.Println(" %s%-25s%s %s", base.ANSI_FG1_GREEN, x.Name, base.ANSI_RESET, x.Description)
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
					a.Help(w)
					w.LineBreak()
					w.Println("")
				}
			})
		})
	}
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
func OptionCommandPrepare(e base.EventDelegate[CommandContext]) CommandOptionFunc {
	return func(ci *commandItem) {
		ci.prepare.Add(e)
	}
}
func OptionCommandRun(e base.EventDelegate[CommandContext]) CommandOptionFunc {
	return func(ci *commandItem) {
		ci.run.Add(e)
	}
}
func OptionCommandClean(e base.EventDelegate[CommandContext]) CommandOptionFunc {
	return func(ci *commandItem) {
		ci.clean.Add(e)
	}
}
func OptionCommandPanic(e base.EventDelegate[error]) CommandOptionFunc {
	return func(ci *commandItem) {
		ci.panic.Add(e)
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

func NewCommandItem(
	category, name, description string,
	options ...CommandOptionFunc,
) CommandItem {
	result := &commandItem{
		CommandDetails: CommandDetails{
			Category:    category,
			Name:        name,
			Description: description,
		},
	}
	result.Options(options...)
	return result
}

func NewCommand(
	category, name, description string,
	options ...CommandOptionFunc,
) (factory func() CommandItem) {
	factory = base.Memoize(func() CommandItem {
		return NewCommandItem(category, name, description, options...)
	})
	AllCommands.FindOrAdd(strings.ToUpper(name), CommandDescriptor{
		Create: factory,
		CommandDetails: CommandDetails{
			Category:    category,
			Name:        name,
			Description: description,
		},
	})
	return
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
	cmds := base.Map(func(it CommandDescriptor) CommandItem {
		return it.Create()
	}, AllCommands.Values()...)
	sort.Slice(cmds, func(i, j int) bool {
		lhs, rhs := cmds[i].Details(), cmds[j].Details()
		return lhs.Compare(rhs) < 0
	})
	return cmds
}
func GetAllCommandNames() []CommandName {
	keys := AllCommands.Keys()
	sort.Strings(keys)
	return base.Map(func(it string) (result CommandName) {
		result.Assign(it)
		return
	}, keys...)
}

func FindCommand(name string) (CommandItem, error) {
	if cmd, found := AllCommands.Get(strings.ToUpper(name)); found {
		return cmd.Create(), nil
	} else {
		return nil, fmt.Errorf("unknown command %q", name)
	}
}

func PrintCommandHelp(w io.Writer, detailed bool) {
	restoreLogLevel := base.GetLogger().SetLevelMaximum(base.LOG_VERBOSE)
	defer base.GetLogger().SetLevel(restoreLogLevel)

	f := base.NewStructuredFile(w, "  ", !detailed)
	pi := GetProcessInfo()

	f.Print(`%v  v.%v  [%v]
build-system for PoPpOlOpPoPo Engine`,
		pi.Path, pi.Version, GetProcessSeed().ShortString())

	header := func(title string) {
		f.Print("%v%v", base.ANSI_FG1_MAGENTA, base.ANSI_FAINT)
		f.Pad(2, "-")
		f.Print(" %s ", title)
		f.Pad(80, "-")
		f.Println("%v", base.ANSI_RESET)
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
			for _, a := range GlobalParsableFlags.arguments {
				f.Println("")
				a.Help(f)
			}
		})

		f.Println("")
	}

	f.Println("%v%vMultiple commands can be executed by using `-and` to join them.", base.ANSI_FG0_MAGENTA, base.ANSI_FAINT)
	f.Println("ex: %s configure -and vscode -and vcxproj -Summary%v", pi.Path, base.ANSI_RESET)
}

/***************************************
 * RunCommands
 ***************************************/

func ParseCommand(cl CommandLine, events *CommandEvents) (err error) {
	name, ok := cl.ConsumeArg(0)
	if !ok {
		return fmt.Errorf("missing command name, use `help` to learn about command usage")
	}

	var cmd CommandItem
	if cmd, err = FindCommand(name); err == nil {
		base.AssertNotIn(cmd, nil)

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
		cmd, _ = FindCommand(x.Command.Get())
	}

	var w io.Writer = base.GetLogger()
	if cmd == nil {
		PrintCommandHelp(w, base.IsLogLevelActive(base.LOG_VERBOSE))
	} else {
		f := base.NewStructuredFile(w, "  ", false)

		f.Println("")
		f.ScopeIndent(func() {
			cmd.Help(f)
		})
	}

	return nil
}

var CommandHelp = NewCommandable(
	"Misc",
	"help",
	"print help about command usage",
	&HelpCommand{})

/***************************************
 * AutoComplete
 ***************************************/

type AutoCompleteCommand struct {
	Command     CommandName
	Inputs      []StringVar
	CompleteArg BoolVar
	Json        BoolVar
	MaxResults  IntVar
}

func (x *AutoCompleteCommand) Flags(cfv CommandFlagsVisitor) {
	cfv.Variable("CompleteArg", "specify that we want to complete a new argument, not command name (even no arguments were given)", &x.CompleteArg)
	cfv.Variable("Json", "output completion results as json, instead of raw text", &x.Json)
	cfv.Variable("MaxResults", "override maximum number of auto-complete results which can be outputed [Default="+x.MaxResults.String()+"]", &x.MaxResults)
}
func (x *AutoCompleteCommand) Init(cc CommandContext) error {
	cc.Options(
		OptionCommandParsableFlags("AutoCompleteCommand", "control autp-completion evaluation", x),
		OptionCommandConsumeArg("command_name", "selected command for auto-completion", &x.Command, COMMANDARG_OPTIONAL),
		OptionCommandConsumeMany("input_text", "text query for command-line auto-completion", &x.Inputs, COMMANDARG_OPTIONAL),
	)
	return nil
}
func (x *AutoCompleteCommand) Run(cc CommandContext) error {
	var autocomplete base.BasicAutoComplete

	command := x.Command.Get()

	inputs := base.MakeStringerSet(x.Inputs...)
	inputs.RemoveAll(`--`)

	for i := len(inputs); i > 0; i-- {
		if inputs[i-1] == `-and` {
			if i < len(inputs) {
				command = inputs[i]
				inputs = inputs[i+1:]
			} else {
				command = ``
				inputs = inputs[i:]
			}
			break
		}
	}

	if len(inputs) == 0 && (len(command) == 0 || !x.CompleteArg.Get()) {
		bg := CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "AutoComplete"}, BUILDGRAPH_QUIET)

		// auto-complete command name
		autocomplete = base.NewAutoComplete(command, x.MaxResults.Get(), bg)
		autocomplete.Append(x.Command /* only for autocomplete */)

		bg.Close()

	} else {
		// auto-complete command arguments or flags
		cmd, err := FindCommand(command)
		if err != nil {
			return err
		}

		input := ""
		if len(inputs) > 0 && !x.CompleteArg.Get() {
			input = inputs[len(inputs)-1]
		}

		bg := CommandEnv.BuildGraph().OpenReadPort(base.ThreadPoolDebugId{Category: "AutoComplete"}, BUILDGRAPH_QUIET)

		autocomplete = base.NewAutoComplete(input, x.MaxResults.Get(), bg)
		autocomplete.Add(`--`, "do not try to parse subsidiary arguments as command-line flags")
		autocomplete.Add(`-and`, "concatenate 2 commands to execute multiple tasks in the same run")
		autocomplete.Append(&GlobalParsableFlags)
		autocomplete.Append(cmd)

		bg.Close()
	}

	base.LogDebug(LogCommand, "auto-complete arguments [%v](%v), complete argument = %v", x.Command, strings.Join(inputs, ", "), x.CompleteArg)
	base.LogVeryVerbose(LogCommand, "auto-complete %q on command-line", autocomplete.GetInput())

	if x.Json.Get() {
		// output autocomplete results as json
		results := autocomplete.GetResults()
		base.JsonSerialize(results, base.GetLogger(), base.OptionJsonPrettyPrint(false))

	} else {
		// output autocomplete results as raw-text
		if base.EnableInteractiveShell() {
			// highlight the matching part of each result if called from an interactive shell
			for _, match := range autocomplete.GetResults() {
				highlighted := autocomplete.Highlight(match.Text, func(r rune) string {
					return fmt.Sprint(base.ANSI_UNDERLINE, base.ANSI_FG1_GREEN, string(r), base.ANSI_RESET)
				})
				base.LogForwardf("%s\t%v%s%v", highlighted, base.ANSI_FG0_CYAN, match.Description, base.ANSI_RESET)
			}
		} else {
			for _, match := range autocomplete.GetResults() {
				base.LogForwardln(match.Text, "\t", match.Description)
			}
		}
	}

	return nil
}

var CommandAutoComplete = NewCommandable(
	"Misc",
	"autocomplete",
	"run auto-completion",
	&AutoCompleteCommand{
		MaxResults: 15,
	})

/***************************************
 * Show Version
 ***************************************/

var CommandBuildVersion = NewCommand(
	"Misc",
	"version",
	"print build version",
	OptionCommandRun(func(cc CommandContext) error {
		base.LogForwardln(GetProcessInfo().String())
		return nil
	}))

/***************************************
 * Show Build Seed
 ***************************************/

var CommandBuildSeed = NewCommand(
	"Misc",
	"seed",
	"print build seed",
	OptionCommandRun(func(cc CommandContext) error {
		base.LogForwardln(GetProcessSeed().String())
		return nil
	}))
