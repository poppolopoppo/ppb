package utils

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
)

var LogCommand = base.NewLogCategory("Command")

var AllCommands = base.SharedMapT[string, func() *commandItem]{}
var AllParsableFlags = base.SharedMapT[string, CommandParsableFlags]{}

var GlobalParsableFlags commandItem

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
		cmd := ci()
		in.Add(cmd.Details().Name, cmd.Description)
	}
}

/***************************************
 * CommandLine
 ***************************************/

type CommandLine interface {
	PeekArg(int) (string, bool)
	ConsumeArg(int) (string, error)
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

func (x *commandLine) String() string {
	return strings.Join(x.args, " ")
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
	var name string
	if name, err = cl.ConsumeArg(0); err != nil {
		return
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
			base.LogTrace(LogCommand, "prepare command %q", it)
			return makeCommandEventErrorIFN(it, "prepare", it.prepare.Invoke(it))
		}))
	}
	if it.run.Bound() {
		x.OnRun.Add(base.AnyDelegate(func() error {
			base.LogTrace(LogCommand, "run command %q", it)
			return makeCommandEventErrorIFN(it, "run", it.run.Invoke(it))
		}))
	}
	if it.clean.Bound() {
		x.OnClean.Add(base.AnyDelegate(func() error {
			base.LogTrace(LogCommand, "clean command %q", it)
			return makeCommandEventErrorIFN(it, "clean", it.clean.Invoke(it))
		}))
	}
	if it.panic.Bound() {
		x.OnPanic.Add(func(err error) error {
			base.LogTrace(LogCommand, "panic command %q: %v", it, err)
			return it.panic.Invoke(err)
		})
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

type CommandArgumentFlags = base.EnumSet[CommandArgumentFlag, *CommandArgumentFlag]

type CommandArgument interface {
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
	Short, Long string
	Description string
	Flags       CommandArgumentFlags
}

func (x *commandBasicArgument) Name() string {
	if len(x.Long) > 0 {
		return x.Long
	}
	return x.Short
}
func (x *commandBasicArgument) HasFlag(flag CommandArgumentFlag) bool {
	return x.Flags.Has(flag)
}
func (x *commandBasicArgument) AutoComplete(in base.AutoComplete) {
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
	w.Print("%s", x.Format())

	if base.EnableInteractiveShell() {
		w.Align(60)
		w.Println("%v%v%s%v", base.ANSI_FG1_BLACK, base.ANSI_FAINT, x.Flags, base.ANSI_RESET)
	}

	w.ScopeIndent(func() {
		w.Println("%v%s%v", base.ANSI_FG0_BLUE, x.Description, base.ANSI_RESET)
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
			Flags:       base.MakeEnumSet(append(flags, COMMANDARG_CONSUME)...),
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
			Flags:       base.MakeEnumSet(append(flags, COMMANDARG_CONSUME, COMMANDARG_VARIADIC)...),
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
	AllParsableFlags.Add(base.GetTypename(parsable), parsable)
	base.RegisterSerializable(&CommandParsableBuilder[T, P]{})
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

func (x *commandParsableArgument) AutoComplete(in base.AutoComplete) {
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
		autocomplete := base.NewAutoComplete("", 8)

		for _, v := range x.Variables {
			colorFG, colorBG := base.ANSI_FG0_CYAN, base.ANSI_BG0_CYAN
			if v.Flags.Has(COMMANDARG_PERSISTENT) {
				colorFG, colorBG = base.ANSI_FG1_MAGENTA, base.ANSI_BG0_RED
			}

			printCommandBullet(w, colorBG)
			w.Print("%v%v-%s%v", base.ANSI_ITALIC, colorFG, v.Name, base.ANSI_RESET)

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

			if err := autocomplete.Any(v.Value); err == nil {
				sb := strings.Builder{}

				sb.WriteString(base.ANSI_FRAME.String())
				sb.WriteString(colorFG.String())
				sb.WriteString(v.Value.String())
				sb.WriteString(base.ANSI_RESET.String())

				sb.WriteString(colorFG.String())
				sb.WriteString(base.ANSI_FAINT.String())
				sb.WriteString(" \t(")
				for i, it := range autocomplete.Results() {
					if i > 0 {
						sb.WriteRune('|')
					}
					sb.WriteString(it.Text)
				}
				sb.WriteRune(')')
				sb.WriteString(base.ANSI_RESET.String())

				w.Println(sb.String())

				autocomplete.ClearResults()
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
			Long:        name,
			Description: description,
			Flags:       base.MakeEnumSet(append(flags, COMMANDARG_OPTIONAL, COMMANDARG_VARIADIC)...),
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
			Flags:  base.MakeEnumSet(COMMANDARG_OPTIONAL),
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
	if flags, ok := AllParsableFlags.Get(x.Name); ok {
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

func GetBuildableFlags[T any, P interface {
	*T
	CommandParsableFlags
}](flags *T) BuildFactoryTyped[*CommandParsableBuilder[T, P]] {
	return MakeBuildFactory(func(bi BuildInitializer) (CommandParsableBuilder[T, P], error) {
		return CommandParsableBuilder[T, P]{
			Name:  base.GetTypename(P(flags)),
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

func (x CommandDetails) GetCommandName() (result CommandName) {
	result.Assign(x.Name)
	return
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
func (x *commandItem) AutoComplete(in base.AutoComplete) {
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
		w.Println(" %s%-20s%s %s", base.ANSI_FG1_GREEN, x.Name, base.ANSI_RESET, x.Description)
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
					printCommandBullet(w, base.ANSI_BG0_YELLOW)
					a.Help(w)
					w.LineBreak()
					w.Println("")
				}
			})
		})
	}
}

func printCommandBullet(w *base.StructuredFile, color base.AnsiCode) {
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

func NewCommand(
	category, name, description string,
	options ...CommandOptionFunc,
) func() CommandItem {
	result := base.Memoize(func() *commandItem {
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
	AllCommands.FindOrAdd(key, result)
	return func() CommandItem {
		if factory, ok := AllCommands.Get(key); ok {
			return factory()
		} else {
			base.LogPanic(LogCommand, "command %q not found", name)
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
	cmds := base.Map(func(it func() *commandItem) *commandItem {
		return it()
	}, AllCommands.Values()...)
	sort.Slice(cmds, func(i, j int) bool {
		if c := strings.Compare(cmds[i].Category, cmds[j].Category); c == 0 {
			return strings.Compare(cmds[i].Name, cmds[j].Name) < 0
		} else {
			return c < 0
		}
	})
	return base.Map(func(it *commandItem) CommandItem { return it }, cmds...)
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
		return cmd(), nil
	} else {
		return nil, fmt.Errorf("unknown command %q", name)
	}
}

func PrintCommandHelp(w io.Writer, detailed bool) {
	restoreLogLevel := base.GetLogger().SetLevelMaximum(base.LOG_VERBOSE)
	defer base.GetLogger().SetLevel(restoreLogLevel)

	f := base.NewStructuredFile(w, "  ", !detailed)

	f.Print(`
%v  v.%v  [%v]
build-system for PoPpOlOpPoPo Engine

  %vcompiled on %v%v`,
		PROCESS_INFO.Path, PROCESS_INFO.Version, GetProcessSeed().ShortString(),
		base.ANSI_FG1_BLACK, PROCESS_INFO.Timestamp.Local(), base.ANSI_RESET)

	header := func(title string) {
		f.Print("%v%v", base.ANSI_FG1_MAGENTA, base.ANSI_FAINT)
		f.Pad(59, "-")
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
}

/***************************************
 * RunCommands
 ***************************************/

func PrepareCommands(cls []CommandLine, events *CommandEvents) error {
	for _, cl := range cls {
		if err := GlobalParsableFlags.Parse(cl); err != nil {
			return err
		}
	}
	events.Add(&GlobalParsableFlags)
	return nil
}

func ParseCommand(cl CommandLine, events *CommandEvents) (err error) {
	var name string
	if name, err = cl.ConsumeArg(0); err != nil {
		return
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

	var w io.Writer = os.Stdout
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

var CommandHelp = NewCommandable("Misc", "help", "print help about command usage", &HelpCommand{})

/***************************************
 * AutoComplete
 ***************************************/

type AutoCompleteCommand struct {
	Command     CommandName
	Inputs      []StringVar
	CompleteArg BoolVar
	MaxResults  IntVar
}

func (x *AutoCompleteCommand) Flags(cfv CommandFlagsVisitor) {
	cfv.Variable("CompleteArg", "specify that we want to complete a new argument, not command name (even no arguments were given)", &x.CompleteArg)
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
	inputs := base.StringSet(base.Stringize(base.RemoveUnless(func(is StringVar) bool {
		return is != "--"
	}, x.Inputs...)...))

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
		// auto-complete command name
		autocomplete = base.NewAutoComplete(command, x.MaxResults.Get())
		autocomplete.Append(x.Command /* only for autocomplete */)

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

		autocomplete = base.NewAutoComplete(input, x.MaxResults.Get())
		autocomplete.Append(&GlobalParsableFlags)
		autocomplete.Append(cmd)
		autocomplete.Add("-and", "concatenate 2 commands to execute multiple tasks in the same run")
	}

	base.LogDebug(LogCommand, "auto-complete arguments [%v](%v), complete argument = %v", x.Command, strings.Join(inputs, ", "), x.CompleteArg)
	base.LogVeryVerbose(LogCommand, "auto-complete %q on command-line", autocomplete.Input())

	for _, match := range autocomplete.Results() {
		if base.EnableInteractiveShell() {
			highlighted := autocomplete.Highlight(match.Text, func(r rune) string {
				return fmt.Sprint(base.ANSI_UNDERLINE, base.ANSI_FG1_GREEN, string(r), base.ANSI_RESET)
			})
			base.LogForwardf("%s\t%v%s%v", highlighted, base.ANSI_FG0_CYAN, match.Description, base.ANSI_RESET)
		} else {
			base.LogForwardln(match.Text, "\t", match.Description)
		}

	}
	return nil
}

var CommandAutoComplete = NewCommandable("Misc", "autocomplete", "run auto-completion", &AutoCompleteCommand{
	MaxResults: 15,
})
