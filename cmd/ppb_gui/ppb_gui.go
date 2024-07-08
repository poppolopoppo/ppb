package main

import (
	"fmt"
	"image/color"
	"slices"
	"sort"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/app"
	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/internal/cmd"
	"github.com/poppolopoppo/ppb/internal/hal"
	"github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
	"golang.org/x/image/colornames"

	"github.com/AllenDang/giu"
)

var LogGui = base.NewLogCategory("Gui")

func getPersistentVarInput(value utils.PersistentVar) giu.Widget {
	switch typed := value.(type) {
	case *utils.BoolVar:
		enabled := typed.Get()
		return giu.Checkbox("Enabled", &enabled).OnChange(func() {
			typed.Assign(enabled)
		})
	case *utils.IntVar:
		n := int32(typed.Get())
		return giu.InputInt(&n).OnChange(func() {
			typed.Assign(int(n))
		})
	case *utils.BigIntVar:
		s := typed.String()
		return giu.InputText(&s).Flags(giu.InputTextFlagsCharsDecimal).OnChange(func() {
			if err := typed.Set(s); err != nil {
				base.LogError(LogGui, "failed to assign value: %v", err)
			}
		})
	case *utils.StringVar:
		s := typed.Get()
		return giu.InputText(&s).OnChange(func() {
			if err := typed.Set(s); err != nil {
				base.LogError(LogGui, "failed to assign value: %v", err)
			}
		})
	case base.EnumSettable:
		flags := base.GatherAutoCompletionFrom(typed)

		s := value.String()

		layout := make(giu.Layout, len(flags))
		for i, flag := range flags {
			enabled := typed.Test(flag.Text)
			layout[i] = giu.Layout{
				giu.Checkbox(flag.Text, &enabled).
					OnChange(func() {
						typed.Select(flag.Text, enabled)
					}),
				giu.Tooltip(flag.Description),
			}
		}

		return giu.ComboCustom(fmt.Sprintf("%T", value), s).Layout(layout)

	default:
		values := base.GatherAutoCompletionFrom(typed)

		if len(values) > 0 {
			strs := base.MakeStringerSet(values...)
			valueStr := value.String()
			selected := int32(slices.Index(strs, valueStr))

			return giu.Combo(fmt.Sprintf("%T", value), valueStr, strs, &selected).OnChange(func() {
				if err := value.Set(strs[selected]); err != nil {
					base.LogError(LogGui, "failed to assign value: %v", err)
				}
			})
		} else {
			s := value.String()
			return giu.InputText(&s).OnChange(func() {
				if err := value.Set(s); err != nil {
					base.LogError(LogGui, "failed to assign value: %v", err)
				}
			})
		}
	}
}

func getConfigTableRows(env *utils.CommandEnvT) (result []*giu.TreeTableRowWidget) {
	objects := env.Persistent().PinObjectNames()
	result = make([]*giu.TreeTableRowWidget, 0, len(objects))

	sort.Strings(objects)

	for _, object := range objects {
		properties, ok := env.Persistent().PinObjectData(object)
		if !ok {
			continue
		}

		children := make([]*giu.TreeTableRowWidget, 0, len(properties))

		keys := base.Keys(properties)
		sort.Strings(keys)

		for _, key := range keys {
			value := properties[key]
			children = append(children, giu.TreeTableRow(key,
				giu.InputText(&value).OnChange(func() {
					str := utils.StringVar(value)
					env.Persistent().StoreData(object, key, &str)
				})))
		}

		result = append(result,
			giu.TreeTableRow(object,
				giu.Label(fmt.Sprintf("%d properties", len(properties)))).
				//Flags(giu.TreeNodeFlagsCollapsingHeader).
				Children(children...))
	}
	return
}

func getCommandTableRows() (result []*giu.TreeTableRowWidget) {
	commands := utils.GetAllCommands()
	result = make([]*giu.TreeTableRowWidget, 0, len(commands))

	for _, cmd := range commands {
		children := make([]*giu.TreeTableRowWidget, 0, len(cmd.Arguments()))

		for _, arg := range cmd.Arguments() {
			details := arg.Details()

			base.LogPanicIfFailed(LogGui, arg.Inspect(func(key utils.CommandArgumentDetails, value utils.PersistentVar) error {
				var textColor color.Color
				if key.Flags.Has(utils.COMMANDARG_PERSISTENT) {
					textColor = colornames.Lightyellow
				} else if key.Flags.Has(utils.COMMANDARG_OPTIONAL) {
					textColor = colornames.Lightgrey
				} else if key.Flags.Has(utils.COMMANDARG_CONSUME) {
					textColor = colornames.Lightseagreen
				} else {
					textColor = colornames.White
				}

				children = append(children, giu.TreeTableRow(key.Long,
					giu.Layout{
						giu.Custom(func() {
							giu.PushColorText(base.NewColorFromStringHash(details.Long))
						}),
						giu.Label(details.Long),
						giu.Tooltip(details.Description),
						giu.Custom(func() {
							giu.PopStyleColor()
						}),
					},
					giu.Layout{
						giu.Custom(func() {
							giu.PushColorText(textColor)
						}),
						getPersistentVarInput(value),
						giu.Tooltip(key.Description),
						giu.Custom(func() {
							giu.PopStyleColor()
						}),
					},
				))
				return nil
			}))
		}

		result = append(result,
			giu.TreeTableRow(cmd.Details().Name,
				giu.Label(cmd.Details().Category),
				giu.Button("Run").OnClick(func() {

				})).
				//Flags(giu.TreeNodeFlagsCollapsingHeader).
				Children(children...))
	}
	return
}

type mainWindow struct {
	env    *utils.CommandEnvT
	wnd    *giu.MasterWindow
	layout giu.Layout
}

func (x *mainWindow) CreateLayout() giu.Layout {
	configPath := x.env.ConfigPath().String()
	databasePath := x.env.DatabasePath().String()

	configTable := getConfigTableRows(x.env)
	commandTable := getCommandTableRows()

	x.layout = giu.Layout{
		giu.TabBar().
			TabItems(
				giu.TabItem("Commands").
					Layout(
						giu.TreeTable().
							Columns(
								giu.TableColumn("Name").InnerWidthOrWeight(5),
								giu.TableColumn("Category").InnerWidthOrWeight(3),
								giu.TableColumn("Params").InnerWidthOrWeight(10),
							).
							Freeze(0, 1).
							Flags(giu.TableFlagsNoBordersInBody|
								giu.TableFlagsSizingStretchProp|
								giu.TableFlagsRowBg|
								giu.TableFlagsScrollY).
							Rows(commandTable...),
					),
				giu.TabItem("Config").Layout(
					giu.Row(
						giu.Label("Path:"),
						giu.InputText(&configPath).Flags(giu.InputTextFlagsReadOnly),
					),
					giu.TreeTable().
						Columns(
							giu.TableColumn("Key"),
							giu.TableColumn("Value"),
						).
						Freeze(0, 1).
						Flags(giu.TableFlagsNoBordersInBody|
							giu.TableFlagsSizingStretchProp|
							giu.TableFlagsRowBg|
							giu.TableFlagsScrollY).
						Rows(configTable...),
				),
				giu.TabItem("Database").Layout(
					giu.Row(
						giu.Label("Path:"),
						giu.InputText(&databasePath).Flags(giu.InputTextFlagsReadOnly),
					),
				),
			),
	}
	return x.layout
}
func newMainWindow(env *utils.CommandEnvT) *mainWindow {
	return &mainWindow{
		env: env,
		wnd: giu.NewMasterWindow("PPB", 600, 600, giu.MasterWindowFlags(0)),
	}
}

func (x *mainWindow) Run() error {
	x.wnd.Run(func() {
		giu.SingleWindow().Layout(x.CreateLayout())
	})
	return nil
}

func main() {
	source, _ := utils.UFS.GetCallerFile(0)
	app.WithCommandEnv("ppe", source, func(env *utils.CommandEnvT) error {
		io.InitIO()
		hal.InitCompile()
		action.InitAction()
		compile.InitCompile()
		cmd.InitCmd()

		return env.Run(func() error {
			wnd := newMainWindow(env)
			return wnd.Run()
		})
	})
}
